// Package flags provides a three-stage feature flag service implementation.
//
// - v0: On/off flags in a watched JSON config file
// - v1: Targeting rules (user_id exact match, email domain, percentage rollout)
// - v2: Append-only audit log + SSE push notifications to subscribers
//
// The public API is designed around a single FlagService struct that grows
// across stages. Each stage adds methods without breaking the previous API.
package flags

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ---------------------------------------------------------------------------
// Shared types
// ---------------------------------------------------------------------------

// EvalContext carries the per-request attributes used for targeting rules.
// Fields that are empty string are considered absent for rule evaluation.
type EvalContext struct {
	UserID     string            `json:"user_id"`
	Email      string            `json:"email"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Rule describes a single targeting condition. Rules are evaluated in order;
// the first rule whose condition matches determines the flag's result for that
// context. If no rule matches, the flag's DefaultEnabled value is used.
//
// Rule types:
//   - "user_id":      Enabled if EvalContext.UserID is in Values list
//   - "email_domain": Enabled if EvalContext.Email has a suffix in Values list (e.g., "@example.com")
//   - "percentage":   Enabled if hash(UserID + FlagName) % 100 < Percentage
type Rule struct {
	Type       string   `json:"type"`
	Values     []string `json:"values,omitempty"`
	Percentage int      `json:"percentage,omitempty"`
	Enabled    bool     `json:"enabled"`
}

// Flag represents a feature flag with optional targeting rules.
// For v0 usage, set DefaultEnabled and leave Rules empty.
// For v1 usage, populate Rules; they are evaluated in order.
type Flag struct {
	Name           string `json:"name"`
	DefaultEnabled bool   `json:"default_enabled"`
	Description    string `json:"description,omitempty"`
	Rules          []Rule `json:"rules,omitempty"`
}

// AuditEntry records a single flag evaluation for audit purposes.
type AuditEntry struct {
	Timestamp time.Time   `json:"timestamp"`
	FlagName  string      `json:"flag_name"`
	UserID    string      `json:"user_id"`
	Result    bool        `json:"result"`
	Context   EvalContext `json:"context"`
}

// ---------------------------------------------------------------------------
// v0: FlagService — on/off flags in a watched JSON file
// ---------------------------------------------------------------------------

// FlagService loads feature flags from a JSON file and keeps them up-to-date
// via fsnotify file watching. All public methods are safe for concurrent use.
//
// The JSON file format is an array of Flag objects:
//
//	[
//	  {"name": "new-checkout", "default_enabled": false, "description": "New checkout flow"},
//	  {"name": "dark-mode",    "default_enabled": true}
//	]
type FlagService struct {
	mu          sync.RWMutex
	flags       map[string]Flag
	configPath  string
	auditPath   string
	auditFile   *os.File
	auditMu     sync.Mutex
	subscribers []chan FlagUpdate
	subMu       sync.RWMutex
}

// FlagUpdate is sent to SSE subscribers when any flag changes.
type FlagUpdate struct {
	FlagName string `json:"flag_name"`
	Flag     Flag   `json:"flag"`
}

// NewFlagService creates a FlagService that loads flags from configPath and
// writes audit events to auditPath. It starts a background goroutine that
// watches configPath for changes and reloads automatically.
//
// Pass auditPath="" to disable audit logging (v0 mode).
func NewFlagService(configPath, auditPath string) (*FlagService, error) {
	fs := &FlagService{
		flags:      make(map[string]Flag),
		configPath: configPath,
		auditPath:  auditPath,
	}

	if err := fs.load(); err != nil {
		return nil, fmt.Errorf("initial load: %w", err)
	}

	if auditPath != "" {
		f, err := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("open audit log: %w", err)
		}
		fs.auditFile = f
	}

	go fs.watchConfig()
	return fs, nil
}

// load reads and parses the JSON config file.
func (fs *FlagService) load() error {
	data, err := os.ReadFile(fs.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file yet — start with empty flag set.
			fs.mu.Lock()
			fs.flags = make(map[string]Flag)
			fs.mu.Unlock()
			return nil
		}
		return err
	}

	var list []Flag
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parse flags.json: %w", err)
	}

	m := make(map[string]Flag, len(list))
	for _, f := range list {
		m[f.Name] = f
	}

	fs.mu.Lock()
	fs.flags = m
	fs.mu.Unlock()
	return nil
}

// watchConfig uses fsnotify to detect changes to configPath and reload.
// Falls back to 5-second polling if fsnotify fails to initialize.
func (fs *FlagService) watchConfig() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("flags: fsnotify unavailable (%v), falling back to 5s poll", err)
		fs.pollConfig()
		return
	}
	defer watcher.Close()

	if err := watcher.Add(fs.configPath); err != nil {
		// File may not exist yet; watch the directory instead.
		dir := "."
		if idx := strings.LastIndex(fs.configPath, "/"); idx >= 0 {
			dir = fs.configPath[:idx]
		}
		if addErr := watcher.Add(dir); addErr != nil {
			log.Printf("flags: cannot watch config dir: %v; falling back to poll", addErr)
			fs.pollConfig()
			return
		}
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if strings.HasSuffix(event.Name, fs.configPath) || event.Name == fs.configPath {
					if err := fs.reload(); err != nil {
						log.Printf("flags: reload error: %v", err)
					}
				}
			}
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("flags: watcher error: %v", watchErr)
		}
	}
}

// pollConfig is the fallback when fsnotify is unavailable.
func (fs *FlagService) pollConfig() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if err := fs.reload(); err != nil {
			log.Printf("flags: poll reload error: %v", err)
		}
	}
}

// reload loads the config and notifies SSE subscribers of any changed flags.
func (fs *FlagService) reload() error {
	// Capture old flags for diff.
	fs.mu.RLock()
	old := make(map[string]Flag, len(fs.flags))
	for k, v := range fs.flags {
		old[k] = v
	}
	fs.mu.RUnlock()

	if err := fs.load(); err != nil {
		return err
	}

	fs.mu.RLock()
	newFlags := fs.flags
	fs.mu.RUnlock()

	// Notify subscribers of every flag that changed.
	for name, newFlag := range newFlags {
		oldFlag, existed := old[name]
		changed := !existed || flagsDiffer(oldFlag, newFlag)
		if changed {
			fs.notify(FlagUpdate{FlagName: name, Flag: newFlag})
		}
	}
	return nil
}

// flagsDiffer returns true if two flags have different configurations.
func flagsDiffer(a, b Flag) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) != string(bj)
}

// IsEnabled returns whether a named flag is enabled, using only the
// DefaultEnabled field (v0 API — no targeting context).
// Returns false for unknown flags (fail-safe default).
func (fs *FlagService) IsEnabled(flagName string) bool {
	fs.mu.RLock()
	f, ok := fs.flags[flagName]
	fs.mu.RUnlock()
	if !ok {
		return false // unknown flag → fail safe
	}
	return f.DefaultEnabled
}

// ---------------------------------------------------------------------------
// v1: Evaluate — targeting rules + percentage rollout
// ---------------------------------------------------------------------------

// Evaluate checks a named flag against targeting rules using the provided
// EvalContext. Rules are evaluated in order; the first matching rule wins.
// If no rules match, Flag.DefaultEnabled is returned.
// Returns false for unknown flags.
//
// Evaluation records an audit entry if an audit log is configured (v2).
func (fs *FlagService) Evaluate(flagName string, ctx EvalContext) bool {
	fs.mu.RLock()
	f, ok := fs.flags[flagName]
	fs.mu.RUnlock()

	if !ok {
		return false // unknown flag → fail safe
	}

	result := fs.evaluateFlag(f, ctx)
	fs.writeAudit(AuditEntry{
		Timestamp: time.Now().UTC(),
		FlagName:  flagName,
		UserID:    ctx.UserID,
		Result:    result,
		Context:   ctx,
	})
	return result
}

// evaluateFlag applies targeting rules to a flag and returns the result.
func (fs *FlagService) evaluateFlag(f Flag, ctx EvalContext) bool {
	for _, rule := range f.Rules {
		if matchRule(rule, f.Name, ctx) {
			return rule.Enabled
		}
	}
	return f.DefaultEnabled
}

// matchRule returns true if the rule condition is satisfied by ctx.
func matchRule(rule Rule, flagName string, ctx EvalContext) bool {
	switch rule.Type {
	case "user_id":
		for _, id := range rule.Values {
			if id == ctx.UserID {
				return true
			}
		}
		return false

	case "email_domain":
		for _, domain := range rule.Values {
			if strings.HasSuffix(ctx.Email, domain) {
				return true
			}
		}
		return false

	case "percentage":
		// Hash user_id + flag_name together so that independent flags produce
		// independent rollout groups. If you only hashed user_id, every 50%
		// flag would include exactly the same set of users — perfect correlation.
		bucket := percentageBucket(ctx.UserID, flagName)
		return bucket < rule.Percentage

	default:
		return false
	}
}

// percentageBucket returns a deterministic integer in [0, 99] for the
// combination of userID and flagName. The same (userID, flagName) pair always
// returns the same bucket, ensuring rollout consistency across evaluations.
func percentageBucket(userID, flagName string) int {
	h := sha256.New()
	h.Write([]byte(userID))
	h.Write([]byte(":"))
	h.Write([]byte(flagName))
	sum := h.Sum(nil)
	// Use the first 8 bytes of the hash as a uint64, then take modulo 100.
	n := binary.BigEndian.Uint64(sum[:8])
	return int(n % 100)
}

// ---------------------------------------------------------------------------
// v1: Flag management (update / list)
// ---------------------------------------------------------------------------

// UpdateFlag replaces a flag's configuration and persists the change to the
// JSON file. Subscribers are notified via SSE.
func (fs *FlagService) UpdateFlag(flag Flag) error {
	fs.mu.Lock()
	old := fs.flags[flag.Name]
	fs.flags[flag.Name] = flag
	list := make([]Flag, 0, len(fs.flags))
	for _, f := range fs.flags {
		list = append(list, f)
	}
	fs.mu.Unlock()

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal flags: %w", err)
	}
	if err := os.WriteFile(fs.configPath, data, 0644); err != nil {
		return fmt.Errorf("write flags.json: %w", err)
	}

	if flagsDiffer(old, flag) {
		fs.notify(FlagUpdate{FlagName: flag.Name, Flag: flag})
	}
	return nil
}

// ListFlags returns a snapshot of all flags.
func (fs *FlagService) ListFlags() []Flag {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	out := make([]Flag, 0, len(fs.flags))
	for _, f := range fs.flags {
		out = append(out, f)
	}
	return out
}

// GetFlag returns a single flag by name, or (Flag{}, false) if not found.
func (fs *FlagService) GetFlag(name string) (Flag, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	f, ok := fs.flags[name]
	return f, ok
}

// ---------------------------------------------------------------------------
// v2: Audit log
// ---------------------------------------------------------------------------

// writeAudit appends a JSON-encoded audit entry to the audit log file.
// No-ops if audit logging is disabled.
func (fs *FlagService) writeAudit(entry AuditEntry) {
	if fs.auditFile == nil {
		return
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	fs.auditMu.Lock()
	defer fs.auditMu.Unlock()
	fs.auditFile.Write(line)
	fs.auditFile.Write([]byte("\n"))
}

// ---------------------------------------------------------------------------
// v2: SSE push notifications
// ---------------------------------------------------------------------------

// Subscribe returns a channel that receives FlagUpdate messages whenever any
// flag changes. The caller must eventually call Unsubscribe to avoid leaks.
// The channel is buffered to prevent slow subscribers from blocking reloads.
func (fs *FlagService) Subscribe() chan FlagUpdate {
	ch := make(chan FlagUpdate, 16)
	fs.subMu.Lock()
	fs.subscribers = append(fs.subscribers, ch)
	fs.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
func (fs *FlagService) Unsubscribe(ch chan FlagUpdate) {
	fs.subMu.Lock()
	defer fs.subMu.Unlock()
	for i, sub := range fs.subscribers {
		if sub == ch {
			fs.subscribers = append(fs.subscribers[:i], fs.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// notify broadcasts a FlagUpdate to all active subscribers.
// Drops the message (non-blocking send) for subscribers whose buffer is full.
func (fs *FlagService) notify(update FlagUpdate) {
	fs.subMu.RLock()
	defer fs.subMu.RUnlock()
	for _, ch := range fs.subscribers {
		select {
		case ch <- update:
		default:
			// Subscriber is not reading — drop rather than block.
		}
	}
}

// Close releases resources held by the FlagService (audit file, watchers).
func (fs *FlagService) Close() error {
	fs.subMu.Lock()
	for _, ch := range fs.subscribers {
		close(ch)
	}
	fs.subscribers = nil
	fs.subMu.Unlock()

	if fs.auditFile != nil {
		return fs.auditFile.Close()
	}
	return nil
}

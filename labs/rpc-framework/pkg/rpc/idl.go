package rpc

// idl.go — v2: simple .rpc IDL parser.
//
// IDL syntax (subset):
//
//	service Calculator {
//	    method Add(AddRequest) returns AddResponse
//	    method Sum(stream AddRequest) returns AddResponse
//	}
//
// The parser produces a ServiceDescriptor containing MethodDescriptors.
// No external tooling (protoc, thrift) is required — the IDL is parsed
// at startup in Go.
//
// Key lesson: a service descriptor is all you need to validate method names
// at registration time, generate stub code, or serve a reflection API.
// gRPC embeds the full protobuf FileDescriptorProto for exactly this reason.

import (
	"bufio"
	"fmt"
	"strings"
)

// MethodDescriptor describes one RPC method.
type MethodDescriptor struct {
	Name         string
	InputType    string
	OutputType   string
	ClientStream bool // true if "stream <InputType>"
	ServerStream bool // reserved for future bidirectional support
}

// ServiceDescriptor describes a complete RPC service parsed from an IDL string.
type ServiceDescriptor struct {
	Name    string
	Methods []MethodDescriptor
}

// ParseIDL parses a .rpc IDL string and returns a ServiceDescriptor.
//
// Supported grammar (whitespace-insensitive):
//
//	service <Name> {
//	    method <Name>( [stream] <InputType> ) returns <OutputType>
//	}
//
// Returns an error if the IDL is malformed.
func ParseIDL(src string) (*ServiceDescriptor, error) {
	scanner := bufio.NewScanner(strings.NewReader(src))
	var desc ServiceDescriptor
	inService := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "service "):
			// "service Calculator {"
			parts := strings.Fields(line)
			if len(parts) < 2 {
				return nil, fmt.Errorf("idl: malformed service declaration: %q", line)
			}
			desc.Name = parts[1]
			inService = true

		case line == "}":
			inService = false

		case inService && strings.HasPrefix(line, "method "):
			// "method Add(AddRequest) returns AddResponse"
			// "method Sum(stream AddRequest) returns AddResponse"
			m, err := parseMethod(line)
			if err != nil {
				return nil, err
			}
			desc.Methods = append(desc.Methods, m)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("idl: scan error: %w", err)
	}
	if desc.Name == "" {
		return nil, fmt.Errorf("idl: no service declaration found")
	}
	return &desc, nil
}

// parseMethod parses a single method line, e.g.
// "method Add(AddRequest) returns AddResponse"
// "method Sum(stream AddRequest) returns AddResponse"
func parseMethod(line string) (MethodDescriptor, error) {
	// Strip "method " prefix.
	line = strings.TrimPrefix(line, "method ")
	line = strings.TrimSpace(line)

	// Split on "(" to get name and rest.
	parenIdx := strings.Index(line, "(")
	if parenIdx < 0 {
		return MethodDescriptor{}, fmt.Errorf("idl: missing '(' in method: %q", line)
	}
	name := strings.TrimSpace(line[:parenIdx])
	rest := line[parenIdx+1:]

	// rest = "AddRequest) returns AddResponse" or "stream AddRequest) returns AddResponse"
	closeIdx := strings.Index(rest, ")")
	if closeIdx < 0 {
		return MethodDescriptor{}, fmt.Errorf("idl: missing ')' in method: %q", line)
	}
	argStr := strings.TrimSpace(rest[:closeIdx])
	afterParen := strings.TrimSpace(rest[closeIdx+1:])

	// Parse optional "stream" keyword.
	clientStream := false
	if strings.HasPrefix(argStr, "stream ") {
		clientStream = true
		argStr = strings.TrimPrefix(argStr, "stream ")
		argStr = strings.TrimSpace(argStr)
	}
	inputType := argStr

	// Parse "returns <OutputType>".
	if !strings.HasPrefix(afterParen, "returns ") {
		return MethodDescriptor{}, fmt.Errorf("idl: expected 'returns' after ')' in: %q", line)
	}
	outputType := strings.TrimSpace(strings.TrimPrefix(afterParen, "returns "))

	return MethodDescriptor{
		Name:         name,
		InputType:    inputType,
		OutputType:   outputType,
		ClientStream: clientStream,
	}, nil
}

// MethodNames returns the set of fully-qualified method names (Service.Method)
// described by this service descriptor.
func (sd *ServiceDescriptor) MethodNames() []string {
	names := make([]string, len(sd.Methods))
	for i, m := range sd.Methods {
		names[i] = sd.Name + "." + m.Name
	}
	return names
}

// Lookup returns the MethodDescriptor for a given method name, or an error if
// the method is not declared in the IDL.
func (sd *ServiceDescriptor) Lookup(methodName string) (MethodDescriptor, error) {
	for _, m := range sd.Methods {
		if m.Name == methodName || sd.Name+"."+m.Name == methodName {
			return m, nil
		}
	}
	return MethodDescriptor{}, fmt.Errorf("rpc: method %q not declared in IDL for service %q", methodName, sd.Name)
}

package ws

// broadcastPresence sends a presence event to all members of the room.
//
// The presence event JSON looks like:
//
//	{"event":"presence","user":"alice","action":"joined","members":["alice","bob"]}
//
// The members list is a snapshot of the room at the time of the event. It is
// sent to every client in the room including the joining client itself.
func broadcastPresence(rm *Room, userID, action string) {
	members := rm.memberIDs()
	msg := marshalServerMsg(serverMessage{
		Event:   "presence",
		Room:    rm.name,
		User:    userID,
		Content: action, // "joined" or "left"
		Members: members,
	})
	rm.broadcast(msg)
}

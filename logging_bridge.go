package main

import (
	"bytes"

	log "github.com/Sirupsen/logrus"
)

// This is a bridge to take the output of Memberlist, which uses a standard
// Go logger and reformat them into properly leveled logrus lines. If
// only the stdlib log.Logger were an interface and not a type...

type LoggingBridge struct{
	testing bool
	lastLevel   []byte
	lastMessage []byte
}

// Memberlist log lines look like:
// 2016/06/24 11:45:33 [DEBUG] memberlist: TCP connection from=172.16.106.1:59598

// Processes one line at a time from the input. If we somehow
// get less than one line in the input, then weird things will
// happen. Experience shows this doesn't currently happen.
func (l *LoggingBridge) Write(data []byte) (int, error) {
	lines := bytes.Split(data, []byte{byte('\n')})

	bytesWritten := len(lines[0])

	fields := bytes.Split(lines[0], []byte{byte(' ')})
	l.logMessageAtLevel(bytes.Join(fields[3:], []byte{byte(' ')}), fields[2])

	return bytesWritten, nil
}

func (l *LoggingBridge) logMessageAtLevel(message []byte, level []byte) {
	if l.testing {
		l.lastLevel = level
		l.lastMessage = message
	}

	switch string(level) {
	case "[INFO]":
		log.Info(string(message))
	case "[WARN]":
		log.Warn(string(message))
	case "[ERR]":
		log.Error(string(message))
	case "[DEBUG]":
		log.Debug(string(message))
	default:
		log.Infof("%s %s", string(level), string(message))
	}
}

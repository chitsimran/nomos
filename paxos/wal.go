package paxos

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"sync"
)

type WAL struct {
	mu   sync.Mutex
	file *os.File
}

func OpenWAL(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &WAL{file: f}, nil
}

func (w *WAL) Append(record WalRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))

	if _, err := w.file.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err := w.file.Write(data); err != nil {
		return err
	}

	return w.file.Sync()
}

func (w *WAL) Replay(apply func(WalRecord) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	for {
		var lenBuf [4]byte
		_, err := io.ReadFull(w.file, lenBuf[:])
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}

		l := binary.BigEndian.Uint32(lenBuf[:])
		data := make([]byte, l)

		_, err = io.ReadFull(w.file, data)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}

		var rec WalRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}

		if err := apply(rec); err != nil {
			return err
		}
	}
}

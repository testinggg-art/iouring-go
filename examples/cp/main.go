package main

import (
	"fmt"
	"os"
	"time"

	"github.com/iceber/iouring-go"
)

const entries uint = 64
const blockSize int64 = 32 * 1024

func main() {
	now := time.Now()

	if len(os.Args) != 3 {
		fmt.Printf("Usage: %s file1 file2\n", os.Args[0])
		return
	}

	iour, err := iouring.New(entries)
	if err != nil {
		panic(fmt.Sprintf("new IOURing error: %v", err))
	}

	src, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Printf("Open src file failed: %v\n", err)
		return
	}
	defer src.Close()

	dest, err := os.Create(os.Args[2])
	if err != nil {
		fmt.Printf("create dest file failed: %v\n", err)
		return
	}
	defer dest.Close()

	if err := iour.RegisterFiles([]*os.File{src, dest}); err != nil {
		panic(err)
	}

	stat, err := src.Stat()
	if err != nil {
		panic(err)
	}
	size := stat.Size()

	var reads int
	var writes int
	var offset uint64

	ch := make(chan *iouring.Result, entries)
	iorequests := make([]iouring.Request, 0, entries)
	for size > 0 {
		if reads >= int(entries) {
			break
		}
		readSize := size
		if readSize > blockSize {
			readSize = blockSize
		}

		b := make([]byte, readSize)
		readRequest := iouring.Pread(int(src.Fd()), b, offset)
		request := iouring.RequestWithInfo(readRequest, offset)
		iorequests = append(iorequests, request)

		size -= readSize
		offset += uint64(readSize)
		reads++
	}

	if err := iour.SubmitRequests(iorequests, ch); err != nil {
		panic(err)
	}

	for comp := 0; comp < reads+writes; comp++ {
		result := <-ch
		if err := result.Err(); err != nil {
			panic(err)
		}

		if result.Opcode() == iouring.IORING_OP_READ {
			b, _ := result.GetRequestBuffer()
			offset := result.GetRequestInfo().(uint64)
			request := iouring.Pwrite(int(dest.Fd()), *b, offset)
			if _, err := iour.SubmitRequest(request, ch); err != nil {
				panic(err)
			}
			writes++
			continue
		}

		if size <= 0 {
			continue
		}

		readSize := size
		if readSize > blockSize {
			readSize = blockSize
		}

		b, _ := result.GetRequestBuffer()
		readRequest := iouring.Pread(int(src.Fd()), (*b)[:readSize], offset)
		request := iouring.RequestWithInfo(readRequest, offset)
		if _, err := iour.SubmitRequest(request, ch); err != nil {
			panic(err)
		}
		size -= readSize
		offset += uint64(readSize)
		reads++
	}
	fmt.Printf("cp successful: %v\n", time.Now().Sub(now))
}

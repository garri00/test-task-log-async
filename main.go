package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Service struct {
	writer         io.Writer
	logCh          chan string
	buffer         []string
	bufferMx       sync.Mutex
	bufferWg       sync.WaitGroup
	bufferNotifyCh chan struct{}
	writeEvery     time.Duration
	writeLimit     int
}

func NewService(writer io.Writer) *Service {
	return &Service{
		writer:         writer,
		logCh:          make(chan string),
		bufferNotifyCh: make(chan struct{}, 1),
		writeEvery:     5 * time.Second, // сливаем логи в writer каждые 5 секунд или 10 записей
		writeLimit:     10,
	}
}

// Run
// У нас есть некий сервис логов. Он принимает логи из разных источников через метод Print и пишет их в io.Writer.
// Проблема в том что io.Writer может не успевать записывать логи так быстро как они пуступают в метод Print.
// Необходимо реализовать сервис таким образом чтобы запись в Print была наиболее быстрой и не была связана с замедленной записью в io.Writer.
// - писать в io.Writer необходимо или каждые 5 секунд или когда накопится 10 записей
// - в io.Writer можно писать весь буфер который доступен в данный момент, но не писать по одной записи
// - Run должен завершаться после закрытия контекста и после завершения всех го-рутин которые он создал
// - после закрытия контекста, если буфер не пустой, его необходимо записать в io.Writer
// - можно добавлять свои методы и поля в Service
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(s.writeEvery)

	for {
		select {
		case <-ctx.Done():
			s.bufferWg.Wait()
			if len(s.buffer) > 0 {
				buff := []byte(strings.Join(s.buffer, "\n") + "\n")
				s.writer.Write(buff)
			}

			close(s.logCh)

			return
		case log := <-s.logCh:
			s.buffer = append(s.buffer, log)

			if len(s.buffer) > s.writeLimit {
				s.bufferNotifyCh <- struct{}{}
			}

		case <-s.bufferNotifyCh:
			if len(s.buffer) > 0 {
				buff := []byte(strings.Join(s.buffer, "\n") + "\n")
				s.buffer = nil

				s.bufferWg.Add(1)
				go func() {
					s.writer.Write(buff)
					s.bufferWg.Done()
				}()
			}

		case <-t.C:
			s.bufferNotifyCh <- struct{}{}
		}
	}

}

func (s *Service) Print(log string, ctx context.Context) {
	// етот метод не завершен
	// тут проблема в том, что после закрытия контекста в Run етот канал не будут читать и запись заблокируется
	// Необходимо чтобы после закрытия контекста етот метот не блокировался. Записать мы уже ничего не можем поетому просто возврат без записи
	//
	if ctx.Err() != nil {
		return
	}

	s.logCh <- log
}

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGHUP)
	//ctx, _ := context.WithTimeout(context.Background(), 15*time.Second) // test context with timeout

	service := NewService(os.Stdout)
	go func() {
		service.Run(ctx)
	}()

	// sends request each second
	go func() {
		t := time.NewTicker(1 * time.Second)
		i := 0

		for {
			select {
			case <-t.C:
				service.Print(fmt.Sprintf("log message %d", i), ctx)
				i++
			case <-ctx.Done():
				return
			}
		}
	}()

	<-ctx.Done()

}

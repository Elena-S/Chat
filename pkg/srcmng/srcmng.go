package srcmng

import (
	"io"
	"sync"

	"github.com/Elena-S/Chat/pkg/logger"
)

type SourceManager interface {
	io.Closer
	MustLaunch()
}

type sourceKeeper struct {
	sources []SourceManager
}

var SourceKeeper sourceKeeper

func (s *sourceKeeper) Add(source SourceManager) {
	s.sources = append(s.sources, source)
}

func (s *sourceKeeper) MustLaunchAll() {
	var wg sync.WaitGroup
	for _, source := range s.sources {
		source := source
		wg.Add(1)
		go func() {
			defer wg.Done()
			source.MustLaunch()
		}()
	}
	wg.Wait()
}

func (s *sourceKeeper) CloseAll() {
	ctxLogger := logger.ChatLogger.WithEventField("Close sources")
	var wg sync.WaitGroup
	for _, source := range s.sources {
		source := source
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := source.Close(); err != nil {
				ctxLogger.Error(err.Error())
			}
		}()
	}
	wg.Wait()
}

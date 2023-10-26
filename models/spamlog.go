package models

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/iotaledger/hive.go/runtime/syncutils"
)

const (
	timeFormat = "2006/01/02 15:04:05"
)

var (
	historyHeader  = "scenario\tstart\tstop\tdeep\treuse\trate\tduration"
	historyLineFmt = "%s\t%s\t%s\t%v\t%v\t%d\t%d\n"
)

type SpammerLog struct {
	spamDetails   []Config
	spamStartTime []time.Time
	spamStopTime  []time.Time
	mu            syncutils.Mutex
}

func NewSpammerLog() *SpammerLog {
	return &SpammerLog{
		spamDetails:   make([]Config, 0),
		spamStartTime: make([]time.Time, 0),
		spamStopTime:  make([]time.Time, 0),
	}
}

func (s *SpammerLog) SpamDetails(spamID int) *Config {
	return &s.spamDetails[spamID]
}

func (s *SpammerLog) StartTime(spamID int) time.Time {
	return s.spamStartTime[spamID]
}

func (s *SpammerLog) AddSpam(config Config) (spamID int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.spamDetails = append(s.spamDetails, config)
	s.spamStartTime = append(s.spamStartTime, time.Now())
	s.spamStopTime = append(s.spamStopTime, time.Time{})

	return len(s.spamDetails) - 1
}

func (s *SpammerLog) SetSpamEndTime(spamID int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.spamStopTime[spamID] = time.Now()
}

func newTabWriter(writer io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(writer, 0, 0, 1, ' ', tabwriter.Debug|tabwriter.TabIndent)
}

func (s *SpammerLog) LogHistory(lastLines int, writer io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w := newTabWriter(writer)
	_, _ = fmt.Fprintln(w, historyHeader)
	idx := len(s.spamDetails) - lastLines + 1
	if idx < 0 {
		idx = 0
	}
	for i, spam := range s.spamDetails[idx:] {
		_, _ = fmt.Fprintf(w, historyLineFmt, spam.Scenario, s.spamStartTime[i].Format(timeFormat), s.spamStopTime[i].Format(timeFormat),
			spam.Deep, spam.Deep, spam.Rate, spam.Duration)
	}
	w.Flush()
}

func (s *SpammerLog) LogSelected(lines []int, writer io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w := newTabWriter(writer)
	_, _ = fmt.Fprintln(w, historyHeader)
	for _, idx := range lines {
		spam := s.spamDetails[idx]
		_, _ = fmt.Fprintf(w, historyLineFmt, spam.Scenario, s.spamStartTime[idx].Format(timeFormat), s.spamStopTime[idx].Format(timeFormat),
			spam.Deep, spam.Deep, spam.Rate, spam.Duration)
	}
	w.Flush()
}

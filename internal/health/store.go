package health

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu      sync.RWMutex
	path    string
	records []Record
	issues  []DataIssue
}

func NewStore(path string, seed []Record, issues []DataIssue) (*Store, error) {
	store := &Store{path: path, issues: issues}
	if path != "" {
		if loaded, err := loadRecords(path); err == nil {
			store.records = loaded
			return store, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	store.records = append([]Record(nil), seed...)
	if path != "" {
		if err := store.saveLocked(); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *Store) All() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := append([]Record(nil), s.records...)
	sortRecords(records)
	return records
}

func (s *Store) Issues() []DataIssue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]DataIssue(nil), s.issues...)
}

func (s *Store) Add(record Record) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record.ID == "" {
		record.ID = fmt.Sprintf("manual-%d-%x", time.Now().UnixNano(), rand.Uint32())
	}
	record.Source = "manual"
	if err := normalizeManualRecord(&record); err != nil {
		return Record{}, err
	}
	if !record.HasMeasurements() {
		return Record{}, errors.New("至少填写一项血糖或血压数据")
	}

	sig := recordSignature(record)
	for _, existing := range s.records {
		if recordSignature(existing) == sig {
			return existing, nil
		}
	}

	s.records = append(s.records, record)
	sortRecords(s.records)
	return record, s.saveLocked()
}

func (s *Store) ImportRecords(records []Record) ([]Record, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing := make(map[string]struct{}, len(s.records))
	for _, record := range s.records {
		existing[recordSignature(record)] = struct{}{}
	}

	imported := make([]Record, 0, len(records))
	skipped := 0
	for index, record := range records {
		if record.ID == "" {
			record.ID = fmt.Sprintf("manual-%d-%03d", time.Now().UnixNano(), index)
		}
		record.Source = "manual"
		if err := normalizeManualRecord(&record); err != nil {
			return nil, skipped, err
		}
		if !record.HasMeasurements() {
			skipped++
			continue
		}
		signature := recordSignature(record)
		if _, ok := existing[signature]; ok {
			skipped++
			continue
		}
		existing[signature] = struct{}{}
		imported = append(imported, record)
		s.records = append(s.records, record)
	}

	sortRecords(s.records)
	return imported, skipped, s.saveLocked()
}

func (s *Store) Replace(record Record) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := normalizeManualRecord(&record); err != nil {
		return Record{}, err
	}
	if !record.HasMeasurements() {
		return Record{}, errors.New("至少填写一项血糖或血压数据")
	}

	for index := range s.records {
		if s.records[index].ID == record.ID {
			if record.Source == "" {
				record.Source = s.records[index].Source
			}
			s.records[index] = record
			sortRecords(s.records)
			return record, s.saveLocked()
		}
	}
	return Record{}, errors.New("记录不存在")
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for index := range s.records {
		if s.records[index].ID == id {
			s.records = append(s.records[:index], s.records[index+1:]...)
			return s.saveLocked()
		}
	}
	return errors.New("记录不存在")
}

func normalizeManualRecord(record *Record) error {
	record.Date = strings.TrimSpace(record.Date)
	record.Time = strings.TrimSpace(record.Time)
	record.Segment = strings.TrimSpace(record.Segment)
	record.Note = strings.TrimSpace(record.Note)
	if record.Date == "" {
		return errors.New("日期不能为空")
	}
	if _, err := time.Parse("2006-01-02", record.Date); err != nil {
		return errors.New("日期格式应为 YYYY-MM-DD")
	}
	if record.Time != "" {
		parsed, err := time.Parse("15:04", record.Time)
		if err != nil {
			return errors.New("时间格式应为 HH:MM")
		}
		record.Segment = segmentFromHour(parsed.Hour())
	}
	if record.Segment == "" {
		record.Segment = "早上"
	}
	return nil
}

func loadRecords(path string) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []Record
	if err := json.NewDecoder(file).Decode(&records); err != nil {
		return nil, err
	}
	sortRecords(records)
	return records, nil
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s.records); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func sortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		left := records[i].Date + " " + records[i].Time + " " + records[i].ID
		right := records[j].Date + " " + records[j].Time + " " + records[j].ID
		return left < right
	})
}

func recordSignature(record Record) string {
	parts := []string{
		record.Date,
		record.Time,
		record.Segment,
		floatSignature(record.DynamicGlucose),
		floatSignature(record.FingerGlucose),
		floatSignature(record.UnknownGlucose),
		intSignature(record.Systolic),
		intSignature(record.Diastolic),
		intSignature(record.Pulse),
		strings.TrimSpace(record.Note),
	}
	return strings.Join(parts, "|")
}

func floatSignature(value *float64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%.3f", *value)
}

func intSignature(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

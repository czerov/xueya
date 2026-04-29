package health

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	dateDotRe        = regexp.MustCompile(`^(\d{4})\.(\d{1,2})\.(\d{1,2})$`)
	dateSpaceRe      = regexp.MustCompile(`^(\d{4})\s+(\d{1,2})\.(\d{1,2})$`)
	dateLongYearRe   = regexp.MustCompile(`^(\d{5})\.(\d{1,2})\.(\d{1,2})$`)
	dateJoinedMonth  = regexp.MustCompile(`^(\d{4})(\d{1,2})\.(\d{1,2})$`)
	timeRe           = regexp.MustCompile(`(\d{1,2}):(\d{2})`)
	dynamicRe        = regexp.MustCompile(`动态\s*(\d+(?:\.\d+)?)`)
	fingerRe         = regexp.MustCompile(`(扎手指|扎手|功手指)\s*(\d+(?:\.\d+)?)`)
	bpRe             = regexp.MustCompile(`血[压圧]\s*(\d{2,3})\s+(\d{2,3})\s+(\d{2,3})`)
	integerRe        = regexp.MustCompile(`\b\d{2,3}\b`)
	decimalRe        = regexp.MustCompile(`(?:^|[^\d])(\d{1,2}\.\d)(?:[^\d]|$)`)
	periodLabelNames = []string{"早上", "中午", "下午", "晚上"}
)

func ParseRaw(raw string) Dataset {
	lines := strings.Split(raw, "\n")
	var records []Record
	var issues []DataIssue
	currentDate := ""
	recordNumber := 0

	for _, line := range lines {
		normalizedLine := normalizeText(line)
		if normalizedLine == "" {
			continue
		}

		if parsedDate, issue, ok := parseDateLine(normalizedLine); ok {
			currentDate = parsedDate
			recordNumber = 0
			if issue != nil {
				issues = append(issues, *issue)
			}
			continue
		}

		if currentDate == "" {
			continue
		}

		recordNumber++
		record, ok := parseRecordLine(currentDate, recordNumber, normalizedLine)
		if ok {
			records = append(records, record)
		}
	}

	sort.SliceStable(records, func(i, j int) bool {
		left := records[i].Date + " " + records[i].Time + " " + records[i].ID
		right := records[j].Date + " " + records[j].Time + " " + records[j].ID
		return left < right
	})

	return Dataset{Records: records, Issues: issues}
}

func normalizeText(value string) string {
	value = strings.ReplaceAll(value, "\u3000", " ")
	value = strings.ReplaceAll(value, "血圧", "血压")
	value = strings.TrimSpace(value)
	return strings.Join(strings.Fields(value), " ")
}

func parseDateLine(line string) (string, *DataIssue, bool) {
	if matches := dateDotRe.FindStringSubmatch(line); matches != nil {
		return normalizeDate(matches[1], matches[2], matches[3], line, "")
	}
	if matches := dateSpaceRe.FindStringSubmatch(line); matches != nil {
		return normalizeDate(matches[1], matches[2], matches[3], line, "日期中年份和月份之间用了空格")
	}
	if matches := dateLongYearRe.FindStringSubmatch(line); matches != nil {
		return normalizeDate(matches[1], matches[2], matches[3], line, "年份位数异常，已按 2026 年整理")
	}
	if matches := dateJoinedMonth.FindStringSubmatch(line); matches != nil {
		return normalizeDate(matches[1], matches[2], matches[3], line, "日期缺少年份和月份之间的点")
	}
	return "", nil, false
}

func normalizeDate(yearText, monthText, dayText, original, message string) (string, *DataIssue, bool) {
	year, _ := strconv.Atoi(yearText)
	month, _ := strconv.Atoi(monthText)
	day, _ := strconv.Atoi(dayText)
	if strings.Contains(yearText, "2026") {
		year = 2026
	}
	if year != 2026 && month >= 1 && month <= 12 {
		year = 2026
		if message == "" {
			message = "年份看起来是手误，已按 2026 年整理"
		}
	}

	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
	if date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return "", nil, false
	}

	fixed := date.Format("2006-01-02")
	if message == "" && original == fmt.Sprintf("%04d.%d.%d", year, month, day) {
		return fixed, nil, true
	}
	if message == "" && yearText == "2026" {
		return fixed, nil, true
	}
	if message == "" {
		message = "日期格式已标准化"
	}
	return fixed, &DataIssue{Original: original, Fixed: fixed, Message: message}, true
}

func parseRecordLine(date string, index int, line string) (Record, bool) {
	record := Record{
		ID:      fmt.Sprintf("%s-%03d", strings.ReplaceAll(date, "-", ""), index),
		Date:    date,
		Segment: segmentFromLabel(line),
		Label:   labelFromLine(line),
		Source:  "imported",
		Raw:     line,
	}

	if matches := timeRe.FindStringSubmatch(line); matches != nil {
		hour, _ := strconv.Atoi(matches[1])
		minute, _ := strconv.Atoi(matches[2])
		record.Time = fmt.Sprintf("%02d:%02d", hour, minute)
		record.Segment = segmentFromHour(hour)
	}

	if matches := dynamicRe.FindStringSubmatch(line); matches != nil {
		record.DynamicGlucose = floatPtr(parseFloat(matches[1]))
	}

	if matches := fingerRe.FindStringSubmatch(line); matches != nil {
		record.FingerGlucose = floatPtr(parseFloat(matches[2]))
		if matches[1] == "功手指" {
			record.Note = appendNote(record.Note, "原文疑似“扎手指”")
		}
	}

	if matches := bpRe.FindStringSubmatch(line); matches != nil {
		record.Systolic = intPtr(parseInt(matches[1]))
		record.Diastolic = intPtr(parseInt(matches[2]))
		record.Pulse = intPtr(parseInt(matches[3]))
	} else if strings.Contains(line, "血压") {
		record.Note = appendNote(record.Note, "血压缺少完整数值")
	}

	if record.Systolic == nil {
		if systolic, diastolic, pulse, ok := findLooseBloodPressure(line); ok {
			record.Systolic = intPtr(systolic)
			record.Diastolic = intPtr(diastolic)
			record.Pulse = intPtr(pulse)
			record.Note = appendNote(record.Note, "原文未写“血压”，已按后三个整数整理")
		}
	}

	if record.DynamicGlucose == nil && record.FingerGlucose == nil {
		if glucose, ok := findUnknownGlucose(line); ok {
			record.UnknownGlucose = floatPtr(glucose)
			record.Note = appendNote(record.Note, "血糖类型未标注")
		}
	}

	if record.Segment == "" {
		record.Segment = "未分段"
	}

	return record, record.HasMeasurements()
}

func (r Record) HasMeasurements() bool {
	return r.DynamicGlucose != nil ||
		r.FingerGlucose != nil ||
		r.UnknownGlucose != nil ||
		r.Systolic != nil ||
		r.Diastolic != nil ||
		r.Pulse != nil
}

func labelFromLine(line string) string {
	for _, label := range periodLabelNames {
		if strings.Contains(line, label) {
			return label
		}
	}
	return ""
}

func segmentFromLabel(line string) string {
	label := labelFromLine(line)
	if label != "" {
		return label
	}
	return "未分段"
}

func segmentFromHour(hour int) string {
	switch {
	case hour >= 0 && hour < 11:
		return "早上"
	case hour >= 11 && hour < 13:
		return "中午"
	case hour >= 13 && hour < 18:
		return "下午"
	default:
		return "晚上"
	}
}

func findLooseBloodPressure(line string) (int, int, int, bool) {
	cleaned := timeRe.ReplaceAllString(line, " ")
	cleaned = dynamicRe.ReplaceAllString(cleaned, " ")
	cleaned = fingerRe.ReplaceAllString(cleaned, " ")
	matches := integerRe.FindAllString(cleaned, -1)
	if len(matches) < 3 {
		return 0, 0, 0, false
	}

	last := matches[len(matches)-3:]
	systolic := parseInt(last[0])
	diastolic := parseInt(last[1])
	pulse := parseInt(last[2])
	if systolic < 70 || systolic > 220 || diastolic < 40 || diastolic > 130 || pulse < 35 || pulse > 160 {
		return 0, 0, 0, false
	}
	return systolic, diastolic, pulse, true
}

func findUnknownGlucose(line string) (float64, bool) {
	cleaned := timeRe.ReplaceAllString(line, " ")
	cleaned = bpRe.ReplaceAllString(cleaned, " ")
	matches := decimalRe.FindStringSubmatch(cleaned)
	if matches == nil {
		return 0, false
	}
	return parseFloat(matches[1]), true
}

func appendNote(existing, next string) string {
	if existing == "" {
		return next
	}
	return existing + "；" + next
}

func parseFloat(value string) float64 {
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func floatPtr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

package health

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

var xlsxHeaders = []string{
	"日期",
	"时间",
	"时间段",
	"动态血糖",
	"扎手指血糖",
	"未标注血糖",
	"收缩压",
	"舒张压",
	"心率",
	"备注",
	"来源",
}

func ExportXLSX(records []Record) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)

	files := map[string]string{
		"[Content_Types].xml":        contentTypesXML,
		"_rels/.rels":                packageRelsXML,
		"xl/workbook.xml":            workbookXML,
		"xl/_rels/workbook.xml.rels": workbookRelsXML,
		"xl/styles.xml":              stylesXML,
		"xl/worksheets/sheet1.xml":   worksheetXML(records),
		"docProps/core.xml":          corePropertiesXML,
		"docProps/app.xml":           appPropertiesXML,
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := writeXLSXFile(archive, name, files[name]); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func ImportXLSX(data []byte) ([]Record, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("无法读取 XLSX 文件")
	}

	sharedStrings, err := readSharedStrings(reader)
	if err != nil {
		return nil, err
	}
	sheet, err := readWorksheet(reader)
	if err != nil {
		return nil, err
	}
	if len(sheet.SheetData.Rows) < 2 {
		return nil, errors.New("XLSX 没有可导入的数据行")
	}

	headerValues := rowValues(sheet.SheetData.Rows[0], sharedStrings)
	headers := make(map[string]int)
	for index, header := range headerValues {
		headers[strings.TrimSpace(header)] = index
	}

	dateIndex, ok := headers["日期"]
	if !ok {
		return nil, errors.New("XLSX 缺少“日期”列")
	}

	var records []Record
	for _, row := range sheet.SheetData.Rows[1:] {
		values := rowValues(row, sharedStrings)
		date, err := normalizeImportedDate(valueAt(values, dateIndex))
		if err != nil {
			continue
		}

		record := Record{
			Date:    date,
			Time:    normalizeImportedTime(valueByHeader(values, headers, "时间")),
			Segment: valueByHeader(values, headers, "时间段"),
			Note:    valueByHeader(values, headers, "备注"),
			Source:  "manual",
		}

		record.DynamicGlucose = optionalFloat(valueByHeader(values, headers, "动态血糖"))
		record.FingerGlucose = optionalFloat(valueByHeader(values, headers, "扎手指血糖"))
		record.UnknownGlucose = optionalFloat(valueByHeader(values, headers, "未标注血糖"))
		record.Systolic = optionalInt(valueByHeader(values, headers, "收缩压"))
		record.Diastolic = optionalInt(valueByHeader(values, headers, "舒张压"))
		record.Pulse = optionalInt(valueByHeader(values, headers, "心率"))

		if record.HasMeasurements() {
			records = append(records, record)
		}
	}

	if len(records) == 0 {
		return nil, errors.New("没有找到可导入的有效记录")
	}
	return records, nil
}

func worksheetXML(records []Record) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	builder.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	builder.WriteString(`<sheetViews><sheetView workbookViewId="0"/></sheetViews>`)
	builder.WriteString(`<sheetFormatPr defaultRowHeight="18"/>`)
	builder.WriteString(`<cols>`)
	builder.WriteString(`<col min="1" max="3" width="14" customWidth="1"/>`)
	builder.WriteString(`<col min="4" max="10" width="16" customWidth="1"/>`)
	builder.WriteString(`<col min="11" max="11" width="12" customWidth="1"/>`)
	builder.WriteString(`</cols>`)
	builder.WriteString(`<sheetData>`)
	builder.WriteString(`<row r="1">`)
	for col, header := range xlsxHeaders {
		writeStringCell(&builder, col, 1, header)
	}
	builder.WriteString(`</row>`)

	for rowIndex, record := range records {
		rowNumber := rowIndex + 2
		builder.WriteString(`<row r="`)
		builder.WriteString(strconv.Itoa(rowNumber))
		builder.WriteString(`">`)
		writeStringCell(&builder, 0, rowNumber, record.Date)
		writeStringCell(&builder, 1, rowNumber, record.Time)
		writeStringCell(&builder, 2, rowNumber, record.Segment)
		writeFloatCell(&builder, 3, rowNumber, record.DynamicGlucose)
		writeFloatCell(&builder, 4, rowNumber, record.FingerGlucose)
		writeFloatCell(&builder, 5, rowNumber, record.UnknownGlucose)
		writeIntCell(&builder, 6, rowNumber, record.Systolic)
		writeIntCell(&builder, 7, rowNumber, record.Diastolic)
		writeIntCell(&builder, 8, rowNumber, record.Pulse)
		writeStringCell(&builder, 9, rowNumber, record.Note)
		writeStringCell(&builder, 10, rowNumber, record.Source)
		builder.WriteString(`</row>`)
	}

	builder.WriteString(`</sheetData>`)
	builder.WriteString(`</worksheet>`)
	return builder.String()
}

func writeStringCell(builder *strings.Builder, col, row int, value string) {
	builder.WriteString(`<c r="`)
	builder.WriteString(cellReference(col, row))
	builder.WriteString(`" t="inlineStr"><is><t>`)
	builder.WriteString(escapeXML(value))
	builder.WriteString(`</t></is></c>`)
}

func writeFloatCell(builder *strings.Builder, col, row int, value *float64) {
	if value == nil {
		writeStringCell(builder, col, row, "")
		return
	}
	builder.WriteString(`<c r="`)
	builder.WriteString(cellReference(col, row))
	builder.WriteString(`"><v>`)
	builder.WriteString(strconv.FormatFloat(*value, 'f', -1, 64))
	builder.WriteString(`</v></c>`)
}

func writeIntCell(builder *strings.Builder, col, row int, value *int) {
	if value == nil {
		writeStringCell(builder, col, row, "")
		return
	}
	builder.WriteString(`<c r="`)
	builder.WriteString(cellReference(col, row))
	builder.WriteString(`"><v>`)
	builder.WriteString(strconv.Itoa(*value))
	builder.WriteString(`</v></c>`)
}

func cellReference(col, row int) string {
	return columnName(col) + strconv.Itoa(row)
}

func columnName(index int) string {
	name := ""
	for index >= 0 {
		name = string(rune('A'+index%26)) + name
		index = index/26 - 1
	}
	return name
}

func escapeXML(value string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(value))
	return buffer.String()
}

func writeXLSXFile(archive *zip.Writer, name, content string) error {
	writer, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(writer, content)
	return err
}

type xlsxWorksheet struct {
	SheetData struct {
		Rows []xlsxRow `xml:"row"`
	} `xml:"sheetData"`
}

type xlsxRow struct {
	Cells []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	Ref    string           `xml:"r,attr"`
	Type   string           `xml:"t,attr"`
	Value  string           `xml:"v"`
	Inline xlsxInlineString `xml:"is"`
}

type xlsxInlineString struct {
	Text string `xml:"t"`
}

type xlsxSharedStrings struct {
	Items []xlsxSharedString `xml:"si"`
}

type xlsxSharedString struct {
	Text string `xml:"t"`
	Runs []struct {
		Text string `xml:"t"`
	} `xml:"r"`
}

func readSharedStrings(reader *zip.Reader) ([]string, error) {
	data, err := readZipEntry(reader, "xl/sharedStrings.xml")
	if errors.Is(err, fsNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var parsed xlsxSharedStrings
	if err := xml.Unmarshal(data, &parsed); err != nil {
		return nil, errors.New("无法解析 sharedStrings.xml")
	}

	values := make([]string, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		if item.Text != "" {
			values = append(values, item.Text)
			continue
		}
		var text strings.Builder
		for _, run := range item.Runs {
			text.WriteString(run.Text)
		}
		values = append(values, text.String())
	}
	return values, nil
}

func readWorksheet(reader *zip.Reader) (xlsxWorksheet, error) {
	data, err := readZipEntry(reader, "xl/worksheets/sheet1.xml")
	if err != nil {
		return xlsxWorksheet{}, errors.New("XLSX 缺少第一张工作表")
	}

	var sheet xlsxWorksheet
	if err := xml.Unmarshal(data, &sheet); err != nil {
		return xlsxWorksheet{}, errors.New("无法解析第一张工作表")
	}
	return sheet, nil
}

var fsNotFound = errors.New("zip entry not found")

func readZipEntry(reader *zip.Reader, name string) ([]byte, error) {
	cleanName := path.Clean(name)
	for _, file := range reader.File {
		if path.Clean(file.Name) != cleanName {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer handle.Close()
		return io.ReadAll(io.LimitReader(handle, 16<<20))
	}
	return nil, fsNotFound
}

func rowValues(row xlsxRow, sharedStrings []string) []string {
	maxColumn := 0
	for index, cell := range row.Cells {
		col := columnIndex(cell.Ref, index)
		if col > maxColumn {
			maxColumn = col
		}
	}

	values := make([]string, maxColumn+1)
	for index, cell := range row.Cells {
		col := columnIndex(cell.Ref, index)
		values[col] = strings.TrimSpace(cellText(cell, sharedStrings))
	}
	return values
}

func cellText(cell xlsxCell, sharedStrings []string) string {
	switch cell.Type {
	case "s":
		index, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err == nil && index >= 0 && index < len(sharedStrings) {
			return sharedStrings[index]
		}
	case "inlineStr":
		return cell.Inline.Text
	}
	return cell.Value
}

func columnIndex(ref string, fallback int) int {
	if ref == "" {
		return fallback
	}
	index := 0
	found := false
	for _, char := range ref {
		if char < 'A' || char > 'Z' {
			break
		}
		found = true
		index = index*26 + int(char-'A'+1)
	}
	if !found {
		return fallback
	}
	return index - 1
}

func normalizeImportedDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("date is empty")
	}
	for _, layout := range []string{"2006-01-02", "2006/1/2", "2006.1.2", "2006/01/02", "2006.01.02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	if serial, err := strconv.ParseFloat(value, 64); err == nil && serial > 30000 {
		date := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(serial))
		return date.Format("2006-01-02"), nil
	}
	return "", fmt.Errorf("invalid date %q", value)
}

func normalizeImportedTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{"15:04", "3:04", "15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("15:04")
		}
	}
	if fraction, err := strconv.ParseFloat(value, 64); err == nil && fraction >= 0 && fraction < 1 {
		totalMinutes := int(math.Round(fraction * 24 * 60))
		hour := totalMinutes / 60
		minute := totalMinutes % 60
		return fmt.Sprintf("%02d:%02d", hour, minute)
	}
	return ""
}

func optionalFloat(value string) *float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func optionalInt(value string) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	parsed := int(math.Round(floatValue))
	return &parsed
}

func valueByHeader(values []string, headers map[string]int, header string) string {
	index, ok := headers[header]
	if !ok {
		return ""
	}
	return valueAt(values, index)
}

func valueAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`

const packageRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const workbookXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="血糖血压记录" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`

const workbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="1"><font><sz val="11"/><name val="Microsoft YaHei"/></font></fonts>
  <fills count="1"><fill><patternFill patternType="none"/></fill></fills>
  <borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>
  <cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`

const corePropertiesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:title>血糖血压记录</dc:title>
  <dc:creator>xueya</dc:creator>
  <cp:lastModifiedBy>xueya</cp:lastModifiedBy>
</cp:coreProperties>`

const appPropertiesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">
  <Application>xueya</Application>
</Properties>`

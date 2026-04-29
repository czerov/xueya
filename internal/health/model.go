package health

type Record struct {
	ID             string   `json:"id"`
	Date           string   `json:"date"`
	Time           string   `json:"time,omitempty"`
	Segment        string   `json:"segment"`
	Label          string   `json:"label,omitempty"`
	DynamicGlucose *float64 `json:"dynamicGlucose,omitempty"`
	FingerGlucose  *float64 `json:"fingerGlucose,omitempty"`
	UnknownGlucose *float64 `json:"unknownGlucose,omitempty"`
	Systolic       *int     `json:"systolic,omitempty"`
	Diastolic      *int     `json:"diastolic,omitempty"`
	Pulse          *int     `json:"pulse,omitempty"`
	Note           string   `json:"note,omitempty"`
	Source         string   `json:"source"`
	Raw            string   `json:"raw,omitempty"`
}

type DataIssue struct {
	Original string `json:"original"`
	Fixed    string `json:"fixed"`
	Message  string `json:"message"`
}

type Dataset struct {
	Records []Record    `json:"records"`
	Issues  []DataIssue `json:"issues"`
}

var SegmentOrder = []string{"早上", "中午", "下午", "晚上"}

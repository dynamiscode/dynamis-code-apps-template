package viewmodel

type ImportPreviewView struct {
	RecordCount int
	Sample      []ImportSampleView
	Issues      []ImportIssueView
	IssueCount  int
	MoreIssues  int
	Digest      string
	Format      string
}

type ImportSampleView struct {
	Title     string
	StatusKey string
	Status    string
}

type ImportIssueView struct {
	Record     int
	MessageKey string
}

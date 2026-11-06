package primitive

import (
	"io"
	"os"
	"testing"
)

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	run()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	printed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(printed)
}

// TestLogPrintsUpToLogLevel pins the threshold of Log: a message prints when its
// level is at most LogLevel, the equal level included.
func TestLogPrintsUpToLogLevel(t *testing.T) {
	original := LogLevel
	t.Cleanup(func() { LogLevel = original })
	cases := []struct {
		logLevel, level int
		want            string
	}{
		{1, 1, "message 7"},
		{2, 1, "message 7"},
		{1, 2, ""},
		{0, 1, ""},
	}
	for _, testCase := range cases {
		LogLevel = testCase.logLevel
		got := captureStdout(t, func() { Log(testCase.level, "message %d", 7) })
		if got != testCase.want {
			t.Errorf("LogLevel %d, level %d: printed %q, want %q", testCase.logLevel, testCase.level, got, testCase.want)
		}
	}
}

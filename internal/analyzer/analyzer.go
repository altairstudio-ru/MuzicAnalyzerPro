package analyzer

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
)

type Analyzer struct {
	PythonBin string
	Script    string
	DB        *sql.DB
}

type AnalysisOutput struct {
	Status         string            `json:"status"`
	ElapsedSeconds float64           `json:"elapsed_seconds"`
	Metrics        []string          `json:"metrics"`
	Results        map[string]any    `json:"results"`
	Errors         []MetricError     `json:"errors,omitempty"`
	Error          string            `json:"error,omitempty"`
}

type MetricError struct {
	Metric string `json:"metric"`
	Error  string `json:"error"`
}

func New(db *sql.DB, pythonBin string) *Analyzer {
	script := ""
	baseDir := findAnalyzerDir()
	if baseDir != "" {
		script = filepath.Join(baseDir, "analyze.py")
	}
	return &Analyzer{
		PythonBin: pythonBin,
		Script:    script,
		DB:        db,
	}
}

func findAnalyzerDir() string {
	candidates := []string{
		"analyzer",
		"../analyzer",
		"../../analyzer",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return ""
}

func (a *Analyzer) Analyze(trackID, audioPath string, metrics []string, lyrics ...string) (*AnalysisOutput, error) {
	if !strings.HasPrefix(audioPath, "/") {
		var err error
		audioPath, err = filepath.Abs(audioPath)
		if err != nil {
			return nil, fmt.Errorf("resolve audio path: %w", err)
		}
	}

	metricsArg := strings.Join(metrics, ",")

	args := []string{a.Script, "--input", audioPath}
	if metricsArg != "" && metricsArg != "all" {
		args = append(args, "--metrics", metricsArg)
	}
	if len(lyrics) > 0 && lyrics[0] != "" {
		args = append(args, "--lyrics", lyrics[0])
	}

	var stderr bytes.Buffer
	cmd := exec.Command(a.PythonBin, args...)
	cmd.Dir = filepath.Dir(a.Script)
	cmd.Stderr = &stderr

	log.Printf("[analyzer] running: %s %s", a.PythonBin, strings.Join(args, " "))
	output, err := cmd.Output()
	if err != nil {
		errMsg := fmt.Sprintf("python error: %v\nstderr: %s", err, stderr.String())
		log.Printf("[analyzer] %s", errMsg)

		ar := &db.AnalysisResult{
			TrackID:    trackID,
			Version:    1,
			Status:     "error",
			ErrorMsg:   errMsg,
			ResultJSON: "{}",
		}
		if ue := db.UpsertAnalysisResult(a.DB, ar); ue != nil {
			log.Printf("[analyzer] upsert error result: %v", ue)
		}
		return nil, errors.New(errMsg)
	}

	var result AnalysisOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse python output: %w\nraw: %s", err, string(output))
	}

	status := "done"
	errMsg := ""
	if result.Status == "error" {
		status = "error"
		errMsg = result.Error
	} else if len(result.Errors) > 0 {
		var parts []string
		for _, e := range result.Errors {
			parts = append(parts, fmt.Sprintf("%s: %s", e.Metric, e.Error))
		}
		errMsg = strings.Join(parts, "; ")
		if errMsg != "" {
			status = "partial"
		}
	}

	jsonBytes, _ := json.Marshal(result)
	ar := &db.AnalysisResult{
		TrackID:    trackID,
		Version:    1,
		Status:     status,
		ErrorMsg:   errMsg,
		ResultJSON: string(jsonBytes),
	}
	if ue := db.UpsertAnalysisResult(a.DB, ar); ue != nil {
		log.Printf("[analyzer] upsert result: %v", ue)
	}

	return &result, nil
}

package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	dockerd "github.com/0206pdh/dockviz-cli/internal/docker"
)

type runResult struct {
	Run              int     `json:"run"`
	SequentialSecond float64 `json:"sequentialSeconds"`
	ParallelSecond   float64 `json:"parallelSeconds"`
	Speedup          float64 `json:"speedup"`
}

type summary struct {
	ContainerCount       int         `json:"containerCount"`
	Runs                 int         `json:"runs"`
	SequentialAvgSeconds float64     `json:"sequentialAvgSeconds"`
	ParallelAvgSeconds   float64     `json:"parallelAvgSeconds"`
	Speedup              float64     `json:"speedup"`
	Results              []runResult `json:"results"`
}

func main() {
	containersFlag := flag.String("containers", "", "comma-separated container names or IDs")
	runsFlag := flag.Int("runs", 5, "number of repeated benchmark runs")
	csvPathFlag := flag.String("csv", "", "CSV output path")
	jsonPathFlag := flag.String("json", "", "JSON summary output path")
	flag.Parse()

	containers := splitCSV(*containersFlag)
	if len(containers) == 0 {
		fatalf("at least one container is required")
	}
	if *runsFlag < 1 {
		fatalf("runs must be >= 1")
	}

	client, err := dockerd.NewClient("")
	if err != nil {
		fatalf("%v", err)
	}
	defer client.Close()

	results := make([]runResult, 0, *runsFlag)
	var sequentialTotal, parallelTotal float64
	for i := 1; i <= *runsFlag; i++ {
		sequential := measureSequential(client, containers)
		parallel := measureParallel(client, containers)
		speedup := sequential / parallel
		result := runResult{
			Run:              i,
			SequentialSecond: round3(sequential),
			ParallelSecond:   round3(parallel),
			Speedup:          round3(speedup),
		}
		results = append(results, result)
		sequentialTotal += sequential
		parallelTotal += parallel
	}

	out := summary{
		ContainerCount:       len(containers),
		Runs:                 *runsFlag,
		SequentialAvgSeconds: round3(sequentialTotal / float64(*runsFlag)),
		ParallelAvgSeconds:   round3(parallelTotal / float64(*runsFlag)),
		Speedup:              round3(sequentialTotal / parallelTotal),
		Results:              results,
	}

	if *csvPathFlag != "" {
		if err := writeCSV(*csvPathFlag, results); err != nil {
			fatalf("write csv: %v", err)
		}
	}
	if *jsonPathFlag != "" {
		if err := writeJSON(*jsonPathFlag, out); err != nil {
			fatalf("write json: %v", err)
		}
	}

	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fatalf("encode summary: %v", err)
	}
	fmt.Println(string(encoded))
}

func splitCSV(value string) []string {
	raw := strings.Split(value, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func measureSequential(client *dockerd.Client, containers []string) float64 {
	start := time.Now()
	for _, container := range containers {
		_, _, _ = client.FetchStats(container)
	}
	return time.Since(start).Seconds()
}

func measureParallel(client *dockerd.Client, containers []string) float64 {
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(len(containers))
	for _, container := range containers {
		container := container
		go func() {
			defer wg.Done()
			_, _, _ = client.FetchStats(container)
		}()
	}
	wg.Wait()
	return time.Since(start).Seconds()
}

func writeCSV(path string, results []runResult) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write([]string{"run", "sequentialSeconds", "parallelSeconds", "speedup"}); err != nil {
		return err
	}
	for _, result := range results {
		if err := writer.Write([]string{
			fmt.Sprintf("%d", result.Run),
			fmt.Sprintf("%.3f", result.SequentialSecond),
			fmt.Sprintf("%.3f", result.ParallelSecond),
			fmt.Sprintf("%.3f", result.Speedup),
		}); err != nil {
			return err
		}
	}
	return writer.Error()
}

func writeJSON(path string, value summary) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}

func round3(value float64) float64 {
	return float64(int(value*1000+0.5)) / 1000
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

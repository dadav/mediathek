package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/gosimple/slug"
	"github.com/mmcdole/gofeed"
)

const (
	version = "0.3.1"
)

var noTrackingFlag bool
var operationalHours string
var trackFile string
var versionFlag bool
var queryString string
var excludeString string
var downloadFlag bool
var outputPath string
var serverModeFlag bool
var queryListPath string
var minimumVideoLenMinutes uint64
var parallelDownloadsNum uint64
var interval uint64

const baseURL string = "https://mediathekviewweb.de/feed?query=%s&everywhere=true&future=false"

var seen []Job
var downloadedFiles []string

func init() {
	flag.BoolVar(&versionFlag, "version", false, "show the current version and exit")
	flag.StringVar(&queryString, "query", "", "search query for the mediathek (use | to separate queries)")
	flag.StringVar(&excludeString, "exclude", "", "exclude query for the found videos (use | to separate excludes)")
	flag.StringVar(&operationalHours, "hours", "", "the hours when the downloads should be executed (separated by comma, e.g. 1,2,3,4)")
	flag.StringVar(&outputPath, "output", "./output", "output path (will be created if not exists)")
	flag.BoolVar(&downloadFlag, "download", false, "download all matches")
	flag.BoolVar(&serverModeFlag, "server", false, "start server mode")
	flag.BoolVar(&noTrackingFlag, "no-track", false, "don't track already downloaded files")
	flag.StringVar(&queryListPath, "query-file", "", "path to a file containing query strings")
	flag.Uint64Var(&minimumVideoLenMinutes, "min", 20, "the minimum video length in minutes")
	flag.Uint64Var(&parallelDownloadsNum, "parallel", uint64(runtime.NumCPU()), "number of parallel downloads")
	flag.Uint64Var(&interval, "interval", 60, "fetch interval in seconds (for server mode)")
	flag.StringVar(&trackFile, "track-file", "~/.mediathek", "location of the track file")

	flag.Parse()
}

// Job contains information about a video
type Job struct {
	url       string
	fileName  string
	outputDir string
}

func loadTrackFile(path string) ([]string, error) {
	var err error
	path, err = expandPath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return []string{}, nil
	}

	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var content []string
	for scanner.Scan() {
		content = append(content, scanner.Text())
	}
	return content, nil
}

func expandPath(dir string) (string, error) {
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, dir[2:]), nil
	}
	return dir, nil
}

func alreadyDownloaded(job Job) bool {
	for _, knownJob := range seen {
		if job == knownJob {
			return true
		}
		if !noTrackingFlag {
			for _, knownURL := range downloadedFiles {
				if job.url == knownURL {
					return true
				}
			}
		}
	}
	return false
}

func fetch(urls []string, excludes []string) {
	var jobs []Job

	for _, url := range urls {
		fp := gofeed.NewParser()
		feed, _ := fp.ParseURL(url)

		for _, item := range feed.Items {
			if videoLen, err := strconv.Atoi(item.Custom["duration"]); err == nil {
				if len(item.Enclosures) > 0 {
					video := item.Enclosures[0]

					if uint64(videoLen/60) >= minimumVideoLenMinutes {
						if serverModeFlag || downloadFlag {

							isExcluded := false
							videoTitle := strings.ToLower(item.Title)
							for _, exclude := range excludes {
								if strings.Contains(videoTitle, strings.ToLower(exclude)) {
									isExcluded = true
									break
								}
							}
							if isExcluded {
								continue
							}

							out, expandErr := expandPath(outputPath)
							if expandErr != nil {
								log.Fatal(expandErr)
							}
							j := Job{
								url:       video.URL,
								fileName:  slug.Make(item.Title),
								outputDir: out,
							}

							if !alreadyDownloaded(j) {
								fmt.Printf("Downloading \"%s\"\n", item.Title)
								seen = append(seen, j)
								jobs = append(jobs, j)
							} else {
								fmt.Printf("Skip: %s (Reason: Already downloaded)\n", item.Title)
							}
						} else {
							fmt.Printf("%s | %d mins\n", item.Title, uint64(videoLen/60))
						}
					}
				}
			}
		}
	}

	if serverModeFlag || downloadFlag {
		if len(jobs) > 0 {
			reqs := make([]*grab.Request, 0)

			for _, job := range jobs {
				if err := os.MkdirAll(job.outputDir, os.ModePerm); err != nil {
					log.Fatal(err)
				}
				dst := filepath.Join(job.outputDir, job.fileName+".mp4")
				req, err := grab.NewRequest(dst, job.url)
				if err == nil {
					reqs = append(reqs, req)
				}
			}

			client := grab.NewClient()
			resp := client.DoBatch(int(parallelDownloadsNum), reqs...)
			for r := range resp {
				if err := r.Err(); err != nil {
					fmt.Println(err)
					continue
				}
				if !noTrackingFlag {
					downloadedFiles = append(downloadedFiles, r.Request.URL().String())
					addToDownloadList(trackFile, r.Request.URL().String())
				}
			}
		}
	}
}

func addToDownloadList(path, url string) error {
	var err error
	path, err = expandPath(path)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(fmt.Sprintf("%s\n", url)); err != nil {
		log.Fatal(err)
	}
	return nil
}

func formatQuery(q string) string {
	return fmt.Sprintf(baseURL, url.QueryEscape(q))
}

func shouldRun(hours string) bool {
	if hours == "" {
		return true
	}

	now := time.Now().Hour()
	for _, hour := range strings.Split(operationalHours, ",") {
		if hourInt, err := strconv.Atoi(hour); err == nil {
			if now == hourInt {
				return true
			}
		}
	}

	return false
}

func main() {
	if versionFlag {
		fmt.Println(version)
		os.Exit(0)
	}

	var excludes []string
	if excludeString != "" {
		for _, singleExcludeString := range strings.Split(excludeString, "|") {
			excludes = append(excludes, singleExcludeString)
		}
	}

	var queries []string

	if queryString != "" {
		for _, singleQueryString := range strings.Split(queryString, "|") {
			queryURL := formatQuery(singleQueryString)
			queries = append(queries, queryURL)
		}
	}

	if queryListPath != "" {
		queryFile, err := os.Open(queryListPath)
		if err != nil {
			log.Fatal(err)
		}
		defer queryFile.Close()

		scanner := bufio.NewScanner(queryFile)
		var q string
		for scanner.Scan() {
			q = formatQuery(strings.TrimSpace(scanner.Text()))
			queries = append(queries, q)
		}
	}

	if queryString == "" && queryListPath == "" {
		fmt.Fprintf(os.Stderr, "Usage of mediathek:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	var err error
	downloadedFiles, err = loadTrackFile(trackFile)
	if err != nil {
		log.Fatal(err)
	}

	if serverModeFlag {
		for {
			if shouldRun(operationalHours) {
				fetch(queries, excludes)
			} else {
				time.Sleep(time.Duration(1) * time.Minute)
				continue
			}

			time.Sleep(time.Duration(interval) * time.Second)
		}
	} else {
		fetch(queries, excludes)
	}
}

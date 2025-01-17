// Small util to get journal info from https://index.pkp.sfu.ca currently
// including 1264043 records indexed from 4960 publications.
//
// https://pkp.sfu.ca/2015/10/23/introducing-the-pkp-index/
//
// Usage:
//
// $ make
// $ ./pkpindex
//
// Output will json lines (oai endpoint is guessed):
//
//     {
//       "name": "Scholarly and Research Communication",
//       "homepage": "http://src-online.ca/index.php/src",
//       "oai": "http://src-online.ca/index.php/src/oai"
//     }
//     {
//       "name": "Stream: Culture/Politics/Technology",
//       "homepage": "http://journals.sfu.ca/stream/index.php/stream",
//       "oai": "http://journals.sfu.ca/stream/index.php/stream/oai"
//     }
//
// Additional ideas:
//
// * check, if journal site is part of a bigger installation (move path element
// up and pattern match).
//
// Notes.
//
// Index page will not yield 404 on invalid page, so max page needs to be set
// manually for now. Pagination seems to require more, maybe cookies.
//
// Pagination is broken, direct link, with custom UA, cookie ends always ends
// up at first page; probably a bit too much JS.
//
// Fetch each journal info page, e.g.
// https://index.pkp.sfu.ca/index.php/browse/archiveInfo/5421 - non-existent
// pages will redirect to homepage, but not via HTTP 3XX, but via "refresh"
// header (http://www.otsukare.info/2015/03/26/refresh-http-header).
//
// Certainly, a site with character.
//
// <div id="content">
// <h3>Revista de Psicologia del Deporte</h3>
// <p class="archiveLinks"><a
// href="https://index.pkp.sfu.ca/index.php/browse/index/37">Browse
// Records</a>&nbsp;&nbsp;|&nbsp;&nbsp;<a href="http://rpd-online.com"
// target="_blank">Journal Website</a>&nbsp;&nbsp;|&nbsp;&nbsp;<a
// href="http://rpd-online.com/issue/current" target="_blank">Current
// Issue</a>&nbsp;&nbsp;|&nbsp;&nbsp;<a
// href="http://rpd-online.com/issue/archive" target="_blank">All
// Issues</a></p>
//
// Let's https://github.com/ericchiang/pup
//
// cat page-000281.html | pup 'h3 text{}' # Journal of Modern Materials
// cat page-000281.html | pup 'p.archiveLinks > a:nth-child(2) attr{href}' # https://journals.aijr.in/index.php/jmm/index
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/sethgrid/pester"
)

const appName = "pkpindex"

var (
	cacheDir               = flag.String("d", path.Join(xdg.CacheHome, appName), "path to cache dir")
	tag                    = flag.String("t", time.Now().Format("2006-01-02"), "subdirectory under cache dir to store pages")
	baseURL                = flag.String("b", "https://index.pkp.sfu.ca/index.php/browse", "base url")
	sleep                  = flag.Duration("s", 1*time.Second, "sleep between requests")
	verbose                = flag.Bool("verbose", false, "verbose output")
	maxID                  = flag.Int("x", 20000, "upper bound, exclusive; max id to fetch")
	userAgent              = flag.String("ua", "Mozilla/5.0 (compatible; MSIE 9.0; Windows NT 6.1; Trident/5.0)", "user agent to use")
	force                  = flag.Bool("f", false, "force redownload of zero length files")
	maxSubsequentRefreshes = flag.Int("mssr", 100, "maximum number of subsequent refreshes")
)

// JournalInfo gathers journal name and endpoint.
type JournalInfo struct {
	Name     string `json:"name"`
	Homepage string `json:"homepage"`
	Endpoint string `json:"oai"`
}

// runPup runs https://github.com/ericchiang/pup selector over html (it does
// not seem to have a convenient programmatic api, https://git.io/Jv09t).
func runPup(html string, selector string) string {
	html = strings.TrimSpace(html)
	if len(html) == 0 {
		return ""
	}
	var buf bytes.Buffer
	cmd := exec.Command("pup", selector)
	cmd.Stdin = strings.NewReader(html)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		log.Printf("runPup failed with %v: %v", err, buf.String())
		return ""
	}
	return strings.TrimSpace(buf.String())
}

// extractJournalInfo extracts name and URL from raw HTML. Be insane and shellout to use pup.
func extractJournalInfo(html string) (*JournalInfo, error) {
	// cat page-000281.html | pup 'h3 text{}' # Journal of Modern Materials
	// cat page-000281.html | pup 'p.archiveLinks > a:nth-child(2) attr{href}' # https://journals.aijr.in/index.php/jmm/index
	name := runPup(html, "h3 text{}")
	homepage := runPup(html, "p.archiveLinks > a:nth-child(2) attr{href}")
	if name == "" {
		log.Printf("empty name for: %s [%d]", html, len(html))
	}
	re := regexp.MustCompile(`/index$`)
	endpoint := ""
	switch {
	case strings.HasSuffix(homepage, "/index"):
		endpoint = re.ReplaceAllString(homepage, "/oai")
	case strings.HasSuffix(homepage, "/"):
		endpoint = homepage + "oai"
	default:
		endpoint = homepage + "/oai"
	}
	return &JournalInfo{
		Name:     name,
		Homepage: homepage,
		Endpoint: endpoint,
	}, nil
}

// extractFromFiles extracts journal info from a list of files.
func extractFromFiles(filenames []string) (result []*JournalInfo, err error) {
	for i, fn := range filenames {
		if i%200 == 0 {
			log.Printf("@%d", i)
		}
		b, err := ioutil.ReadFile(fn)
		if err != nil {
			return result, err
		}
		ji, err := extractJournalInfo(string(b))
		if err != nil {
			return result, err
		}
		result = append(result, ji)
	}
	return result, nil
}

func main() {
	flag.Parse()
	// Create target directory.
	target := path.Join(*cacheDir, *tag)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		if err := os.MkdirAll(target, 0755); err != nil {
			log.Fatal(err)
		}
	}
	client := pester.New()
	client.SetRetryOnHTTP429(true)
	id := 0
	subsequentRefreshes := 0
	for i := 0; i < *maxID; i++ {
		// wrapFunc, so we can enjoy the defer on resp.Body.
		wrapFunc := func() {
			if subsequentRefreshes > *maxSubsequentRefreshes {
				return
			}
			id++
			// https: //index.pkp.sfu.ca/index.php/browse/archiveInfo/5000
			link := fmt.Sprintf("%s/archiveInfo/%d", *baseURL, id)
			filename := fmt.Sprintf("page-%06d.html", id)
			dst := path.Join(target, filename)
			if fi, err := os.Stat(dst); err == nil {
				if fi.Size() > 0 || !*force {
					log.Printf("already cached %s %s", dst, link)
					return
				}
				if *verbose {
					log.Printf("force redownload: %s", link)
				}
			}
			resp, err := client.Get(link)
			if err != nil {
				log.Fatal(err)
			}
			if resp.StatusCode >= 400 {
				log.Fatalf("failed with %s", resp.Status)
			}
			defer resp.Body.Close()
			// refresh: 0; url=https://index.pkp.sfu.ca/index.php/browse
			refresh := resp.Header.Get("refresh")
			if refresh != "" {
				log.Printf("[touch] refresh found for %s", link)
				// Just touch.
				if err := WriteFileAtomic(dst, []byte{}, 0644); err != nil {
					log.Fatal(err)
				}
				subsequentRefreshes++
				return
			}
			subsequentRefreshes = 0
			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Fatal(err)
			}
			if err := WriteFileAtomic(dst, b, 0644); err != nil {
				log.Fatal(err)
			}
			if *verbose {
				log.Printf("done: %s %s", dst, link)
			}
			time.Sleep(*sleep)
		}
		wrapFunc()
	}
	// Find all files and extract journal info.
	var files []string
	err := filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if info.Size() == 0 {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	if *verbose {
		log.Printf("extracting journal info from %d files", len(files))
	}
	infos, err := extractFromFiles(files)
	if err != nil {
		log.Fatal(err)
	}
	// Write out JSON.
	for _, info := range infos {
		b, err := json.Marshal(info)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(b))
	}
}

// WriteFileAtomic writes the data to a temp file and atomically move if everything else succeeds.
func WriteFileAtomic(filename string, data []byte, perm os.FileMode) error {
	dir, name := path.Split(filename)
	f, err := ioutil.TempFile(dir, name)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if permErr := os.Chmod(f.Name(), perm); err == nil {
		err = permErr
	}
	if err == nil {
		err = os.Rename(f.Name(), filename)
	}
	// Any err should result in full cleanup.
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

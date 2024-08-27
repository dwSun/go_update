package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/schollz/progressbar/v3"
)

var src = "https://golang.google.cn/dl/"

var os_str = runtime.GOOS
var arch_str = runtime.GOARCH

func main() {
	fmt.Println("Current OS: " + os_str)
	fmt.Println("Current Arch: " + arch_str)

	suffix := fmt.Sprintf(".%s-%s.tar.gz", os_str, arch_str)

	ver_latest, sha, err := GotLatestVersion()

	if err != nil {
		fmt.Println(err)
		return
	}

	ver_current := GetCurrent()

	if ver_current != ver_latest {
		fmt.Printf("New Version [%s] Found! Current [%s]\n", ver_latest, ver_current)
		// 暂停5秒
		time.Sleep(5 * time.Second)

		latest_gz := ver_latest + suffix
		latest_url := src + latest_gz
		fmt.Println("Download " + latest_url)

		if FileExist(latest_gz) && CheckSHA256(latest_gz, sha) {
			//file OK
		} else {
			err := downloadFile(latest_gz, latest_url)
			if err != nil {
				panic(err)
			}
			fmt.Println("Download Finished")
		}
		os.RemoveAll("go")
		DeCompress(latest_gz)

		fmt.Println("Update Finished")

	} else {
		fmt.Printf("Latest Version [%s]. Current [%s]. Skip\n", ver_latest, ver_current)
	}
	PrintPathSet()
}

func CheckSHA256(latest_gz string, sha string) bool {
	file, err := os.Open(latest_gz)
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		fmt.Println(err)
		return false
	}

	if fmt.Sprintf("%x", h.Sum(nil)) == sha {
		fmt.Println("SHA256 Check Passed")
		return true
	} else {
		fmt.Println("SHA256 Check Failed")
		os.Remove(latest_gz)
		return false
	}
}

func GetIndexSrc() ([]byte, error) {
	resp, err := http.Get(src)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return body, err
}

func PrintPathSet() {
	work_path, _ := os.Getwd()
	go_root := work_path + "/go"
	fmt.Println("export GOROOT=" + go_root)
	fmt.Println("export PATH=$GOROOT/bin:$PATH")
	home, _ := os.UserHomeDir()
	go_path := home + "/go"
	fmt.Println("export GOPATH=" + go_path)
	fmt.Println("export PATH=$GOPATH/bin:$PATH")
	fmt.Println("export GO111MODULE=auto")
	fmt.Println("export GOPROXY=https://goproxy.cn,direct")
}

func FileExist(path string) bool {
	_, err := os.Lstat(path)
	return !os.IsNotExist(err)
}

func GetCurrent() string {
	// 查找环境变量中 go可执行文件的位置

	cmd := exec.Command("go", "version")
	out, err := cmd.Output()
	if err != nil {
		fmt.Println(err)
		return ""
	}
	ver := string(strings.Split(string(out), " ")[2])
	return ver

}

func downloadFile(filepath string, url string) error {
	req, _ := http.NewRequest("GET", url, nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode == 200 {
		defer resp.Body.Close()

		f, _ := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0644)
		defer f.Close()

		bar := progressbar.DefaultBytes(
			resp.ContentLength,
			"Downloading",
		)
		io.Copy(io.MultiWriter(f, bar), resp.Body)
	} else {
		fmt.Println(resp.StatusCode)
	}

	return nil
}

func DeCompress(tarFile string) error {
	srcFile, err := os.Open(tarFile)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	gr, err := gzip.NewReader(srcFile)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return err
			}
		}
		filename := hdr.Name

		fmt.Println(filename)
		file, err := createFile(filename)
		if err != nil {
			fmt.Println(err)
		} else {
			io.Copy(file, tr)
			file.Close()
			os.Chmod(filename, os.FileMode(hdr.Mode))
		}
	}
	return nil
}

func createFile(name string) (*os.File, error) {
	err := os.MkdirAll(string([]rune(name)[0:strings.LastIndex(name, "/")]), 0755)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return os.Create(name)
}

func GotLatestVersion() (string, string, error) {

	body, err := GetIndexSrc()
	if err != nil {
		return "", "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		panic(err)
	}

	archive_suffix := fmt.Sprintf(".%s-%s.tar.gz", os_str, arch_str)
	r_archive, _ := regexp.Compile(`/dl/(go[\d+][\.\d+]*)` + archive_suffix)

	ver := ""
	sha256 := ""

	doc.Find("table").Each(func(i int, s *goquery.Selection) {
		s.Find("tr").Each(func(i int, s *goquery.Selection) {

			theHTml, err := s.Html()
			if err != nil {
				fmt.Println(err)
				return
			}

			if strings.Contains(theHTml, "Archive") {
				match_archive := r_archive.MatchString(string(theHTml))
				if match_archive {
					strs := r_archive.FindStringSubmatch(string(theHTml))

					if ver == "" {
						ver = strs[1]
						fmt.Println("Current Version: " + ver)

					}

					s.Find("tt").Each(func(i int, s *goquery.Selection) {
						if sha256 == "" {
							sha256 = s.Text()
							fmt.Println("SHA256: " + sha256)
						}
					})

					return
				}
			}
		})
	})

	return ver, sha256, nil

}

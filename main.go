package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
)

var src = "https://go.dev/dl/"
var host = "https://go.dev"

func main() {
	resp, err := http.Get(src)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	// 判断当前系统是 linux 还是mac
	os_str := runtime.GOOS
	fmt.Println("Current OS: " + os_str)
	arch_str := runtime.GOARCH
	fmt.Println("Current Arch: " + arch_str)

	suffix := fmt.Sprintf(".%s-%s.tar.gz", os_str, arch_str)

	r, _ := regexp.Compile(`/dl/(go[\d+][\.\d+]*)` + suffix)
	match := r.MatchString(string(body))
	if match {
		strs := r.FindStringSubmatch(string(body))
		ver_current := GetCurrent()

		ver_latest := strs[1]
		if ver_current != ver_latest {
			fmt.Printf("New Version [%s] Found! Current [%s]\n", ver_latest, ver_current)
			// 暂停5秒
			time.Sleep(5 * time.Second)
			latest_url := host + strs[0]
			fmt.Println("Download " + latest_url)

			latest_gz := ver_latest + suffix

			if FileExist(latest_gz) {
				fmt.Println("File Exist")
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
			PrintPathSet()

		} else {
			fmt.Printf("Latest Version [%s]. Current [%s]. Skip\n", ver_latest, ver_current)
		}
	}
	fmt.Println()
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

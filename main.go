// go_update：检测并下载最新 Go 发行版，解压到当前目录的 go/ 下，最后打印环境变量配置提示。
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/schollz/progressbar/v3"
)

const downloadURL = "https://go.dev/dl/"

var (
	goos          = runtime.GOOS
	goarch        = runtime.GOARCH
	archiveSuffix = fmt.Sprintf(".%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
)

func main() {
	fmt.Printf("Current OS: %s\nCurrent Arch: %s\n", goos, goarch)

	latest, sha, err := getLatestVersion()
	if err != nil {
		fmt.Println(err)
		return
	}

	current := currentGoVersion()
	if current == latest {
		fmt.Printf("Latest Version [%s]. Current [%s]. Skip\n", latest, current)
		printEnvHints()
		return
	}

	// 发现新版本，稍等片刻后开始下载
	fmt.Printf("New Version [%s] Found! Current [%s]\n", latest, current)
	time.Sleep(5 * time.Second)

	archive := latest + archiveSuffix
	url := downloadURL + archive
	fmt.Println("Download " + url)

	if fileExists(archive) && checkSHA256(archive, sha) {
		// 本地已有且校验通过，跳过下载
	} else if err := downloadFile(archive, url); err != nil {
		panic(err)
	} else {
		fmt.Println("Download Finished")
	}

	os.RemoveAll("go")
	decompress(archive) // 与原逻辑一致：忽略解压错误

	fmt.Println("Update Finished")
	printEnvHints()
}

// --- 版本查询 ---

// getLatestVersion 从 go.dev/dl 页面解析当前平台的最新版本号与 SHA256。
func getLatestVersion() (ver, sha string, err error) {
	body, err := fetchDownloadPage()
	if err != nil {
		return "", "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		panic(err)
	}

	// 正则与原实现保持一致（含 [\d+] 字符类）
	re := regexp.MustCompile(`/dl/(go[\d+][\.\d+]*)` + archiveSuffix)

	doc.Find("table tr").Each(func(_ int, row *goquery.Selection) {
		if ver != "" {
			return
		}
		html, err := row.Html()
		if err != nil {
			fmt.Println(err)
			return
		}
		if !strings.Contains(html, "Archive") {
			return
		}
		m := re.FindStringSubmatch(html)
		if m == nil {
			return
		}
		ver = m[1]
		fmt.Println("Current Version: " + ver)
		row.Find("tt").Each(func(_ int, tt *goquery.Selection) {
			if sha == "" {
				sha = tt.Text()
				fmt.Println("SHA256: " + sha)
			}
		})
	})

	return ver, sha, nil
}

// currentGoVersion 读取 PATH 中 go 的版本号；未安装时返回空字符串。
func currentGoVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		fmt.Println(err)
		return ""
	}
	return strings.Split(string(out), " ")[2]
}

func fetchDownloadPage() ([]byte, error) {
	resp, err := http.Get(downloadURL)
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
	return body, nil
}

// --- 下载与校验 ---

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return !os.IsNotExist(err)
}

func checkSHA256(path, want string) bool {
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		fmt.Println(err)
		return false
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != want {
		fmt.Println("SHA256 Check Failed")
		os.Remove(path)
		return false
	}
	fmt.Println("SHA256 Check Passed")
	return true
}

func downloadFile(dest, url string) error {
	req, _ := http.NewRequest("GET", url, nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode == 200 {
		defer resp.Body.Close()

		f, _ := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0644)
		defer f.Close()

		bar := progressbar.DefaultBytes(resp.ContentLength, "Downloading")
		io.Copy(io.MultiWriter(f, bar), resp.Body)
	} else {
		fmt.Println(resp.StatusCode)
	}
	return nil
}

// --- 解压 ---

func decompress(tarFile string) error {
	fi, err := os.Stat(tarFile)
	if err != nil {
		return err
	}

	total, err := gzipUncompressedSize(tarFile, fi)
	if err != nil || total == 0 {
		total = fi.Size() * 5 // ponytail: ISIZE 不可用时按压缩比粗估
	}

	f, err := os.Open(tarFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	bar := progressbar.DefaultBytes(total, "Extracting")
	defer func() { _ = bar.Finish() }()

	tr := tar.NewReader(&barReader{r: gr, bar: bar})
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := extractTarEntry(tr, hdr); err != nil {
			return fmt.Errorf("%s: %w", hdr.Name, err)
		}
	}
	return nil
}

// extractTarEntry 按 tar 条目类型写入对应文件系统对象。
func extractTarEntry(tr *tar.Reader, hdr *tar.Header) error {
	name := hdr.Name
	mode := os.FileMode(hdr.Mode)

	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(name, mode)
	case tar.TypeReg, tar.TypeRegA:
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			return err
		}
		return os.Symlink(hdr.Linkname, name)
	case tar.TypeLink:
		return os.Link(hdr.Linkname, name)
	default:
		if hdr.Size > 0 {
			_, err := io.CopyN(io.Discard, tr, hdr.Size)
			return err
		}
		return nil
	}
}

// barReader 在读取 gzip 流时同步更新进度条。
type barReader struct {
	r   io.Reader
	bar *progressbar.ProgressBar
}

func (b *barReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if n > 0 {
		b.bar.Add(n)
	}
	return n, err
}

// gzipUncompressedSize 读取 gzip 尾部的 ISIZE 字段（未压缩大小 mod 2^32）。
func gzipUncompressedSize(path string, fi os.FileInfo) (int64, error) {
	if fi.Size() < 4 {
		return 0, fmt.Errorf("file too small")
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var buf [4]byte
	if _, err := f.ReadAt(buf[:], fi.Size()-4); err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint32(buf[:])), nil
}

// --- 输出 ---

// printEnvHints 打印写入 shell profile 所需的环境变量 export 语句。
func printEnvHints() {
	workPath, _ := os.Getwd()
	goRoot := workPath + "/go"
	fmt.Println("🎉 Update Finished")
	fmt.Println("================================================")
	fmt.Println("export GOROOT=" + goRoot)
	fmt.Println("export PATH=$GOROOT/bin:$PATH")

	home, _ := os.UserHomeDir()
	fmt.Println("export GOPATH=" + home + "/go")
	fmt.Println("export PATH=$GOPATH/bin:$PATH")
	fmt.Println("export GO111MODULE=auto")
	fmt.Println("export GOPROXY=https://goproxy.cn,direct")
}

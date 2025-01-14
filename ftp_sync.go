package main

import (
	"context"
	"errors"
	"fmt"
	"ftp_sync/goftp"
	"github.com/cheggaaa/pb/v3"
	httpdialer "github.com/mwitkow/go-http-dialer"
	"math"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

//goland:noinspection GoUnusedConst,GoSnakeCaseUsage
const (
	MAX_TRY      = 10
	WAIT_SECONDS = 10
	WGET_TIMEOUT = 5 * time.Minute
)

//goland:noinspection GoSnakeCaseUsage
type FTP_SYNC struct {
	Host            string
	UserName        string
	Password        string
	LocalDirectory  string
	RemoteDirectory string
	ExcludePaths    []string
	ListFilesProxy  string
	DownloadProxy   string
}

//goland:noinspection GoSnakeCaseUsage
type FTP_FILE_INFO struct {
	name string
	path string
	size int64
}

//goland:noinspection GoSnakeCaseUsage
type FILE_INFO_JSON []struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

//goland:noinspection GoUnhandledErrorResult
func (sync FTP_SYNC) Synchronization(host string, userName string, password string, localDirectory string, remoteDirectory string) {
	sync.Host = host
	sync.UserName = userName
	sync.Password = password
	sync.LocalDirectory = localDirectory
	sync.RemoteDirectory = remoteDirectory
	localDirectory = strings.TrimRight(localDirectory, "/")
	localDirectory = strings.TrimRight(localDirectory, "\\")
	remoteDirectory = strings.TrimRight(remoteDirectory, "/")
	remoteDirectory = strings.TrimRight(remoteDirectory, "\\")
	sync.listFiles(remoteDirectory, func(ftpFile FTP_FILE_INFO) {
		isMissing := false
		ftpFilePath := ftpFile.path
		ftpFilePath = strings.TrimLeft(ftpFilePath, "/")
		ftpFilePath = strings.TrimLeft(ftpFilePath, "\\")
		ftpFilePath = ftpFilePath[len(remoteDirectory):]
		saveTo := ""
		for _, part := range strings.Split(fmt.Sprintf("%s%c%s", localDirectory, os.PathSeparator, strings.Replace(ftpFilePath, "/", "\\", -1)), "\\") {
			if len(saveTo) > 0 {
				saveTo += "\\"
			}
			for strings.HasSuffix(part, " ") {
				part = part[:len(part)-1]
			}
			saveTo += part
		}
		stat, err := os.Stat(saveTo)
		if os.IsNotExist(err) {
			isMissing = true
		} else if err == nil {
			isMissing = stat.Size() != ftpFile.size
		}
		if isMissing {
			localFileSize := "NOT FOUND"
			if stat != nil {
				localFileSize = fmt.Sprintf("%d", stat.Size())
			}
			ftpFilePath := strings.Replace(fmt.Sprintf("ftp://%s%s", host, ftpFile.path), "\\", "/", -1)
			len1 := len(saveTo)
			len2 := len(ftpFilePath)
			space := strings.Repeat(" ", int(math.Abs(float64(len1-len2))))
			fmt.Println()
			if len1 > len2 {
				fmt.Printf("[%s] \033[97m%s\033[0m: \033[91m%s\033[0m\n[%s] \033[97m%s\033[0m:%s \033[92m%d\033[0m\n", sync.Now(), saveTo, localFileSize, sync.Now(), ftpFilePath, space, ftpFile.size)
			} else {
				fmt.Printf("[%s] \033[97m%s\033[0m:%s \033[91m%s\033[0m\n[%s] \033[97m%s\033[0m: \033[92m%d\033[0m\n", sync.Now(), saveTo, space, localFileSize, sync.Now(), ftpFilePath, ftpFile.size)
			}
			sync.wget(ftpFile.path, saveTo)
		} else {
			fmt.Printf("[%s] \033[97m%s\033[0m is \033[92mup to date\033[0m\n", sync.Now(), saveTo)
		}
	})
}

//goland:noinspection GoUnhandledErrorResult
func (sync FTP_SYNC) CreateClientWithProxy(proxyUrl string) (client *goftp.Client, err error) {
	dialFunc := func(network string, address string) (conn net.Conn, err error) {
		var proxyURL *url.URL
		if proxyURL, err = url.Parse(proxyUrl); err == nil {
			conn, err = httpdialer.New(proxyURL).Dial(network, address)
		}
		return
	}
	if proxyUrl == "" {
		dialFunc = nil
	}
	client, err = goftp.DialConfig(goftp.Config{
		User:        sync.UserName,
		Password:    sync.Password,
		DisableEPSV: true,
		DialFunc:    dialFunc,
	}, sync.Host)
	return
}

func (sync FTP_SYNC) CreateClient() (client *goftp.Client, err error) {
	return sync.CreateClientWithProxy("")
}

//goland:noinspection GoUnhandledErrorResult
func (sync FTP_SYNC) listFiles(remoteDirectory string, fileInfo func(ftpFile FTP_FILE_INFO)) {
	directory := remoteDirectory
	directory = strings.TrimRight(directory, "/")
	directory = strings.TrimRight(directory, "\\")
	if directory == "" {
		directory = "/"
	}
	isExclude := false
	if sync.ExcludePaths != nil {
		for _, exclude := range sync.ExcludePaths {
			if strings.EqualFold(directory, exclude) {
				isExclude = true
			}
		}
	}
	if !isExclude {
		for retryCount := MAX_TRY; retryCount > 0; retryCount-- {
			var client *goftp.Client
			var err error
			if sync.ListFilesProxy == "" {
				client, err = sync.CreateClient()
			} else {
				client, err = sync.CreateClientWithProxy(sync.DownloadProxy)
			}
			if err == nil {
				if files, err := client.ReadDir(directory); err == nil {
					for _, file := range files {
						fullFilePath := filepath.Join(directory, file.Name())
						if file.IsDir() {
							sync.listFiles(fullFilePath, fileInfo)
						} else {
							if fileInfo != nil {
								fileInfo(FTP_FILE_INFO{
									name: file.Name(),
									path: fullFilePath,
									size: file.Size(),
								})
							}
						}
					}
					break
				} else {
					fmt.Printf("[%s] List files from \033[97m%s\033[0m \033[91mfailed\033[0m\n", sync.Now(), sync.Host)
				}
				client.Close()
			}
			fmt.Printf("[%s] Wait %d seconds...\n", sync.Now(), WAIT_SECONDS)
			time.Sleep(WAIT_SECONDS * time.Second)
		}
	}
	return
}

//goland:noinspection GoUnhandledErrorResult,GoDfaErrorMayBeNotNil
func (sync FTP_SYNC) DownloadFile(ftpFilePath string, saveTo string) (err error) {
	for retryCount := MAX_TRY; retryCount > 0; retryCount-- {
		var client *goftp.Client
		if sync.DownloadProxy == "" {
			client, err = sync.CreateClient()
		} else {
			client, err = sync.CreateClientWithProxy(sync.DownloadProxy)
		}
		if err == nil {
			if err = os.MkdirAll(filepath.Dir(saveTo), os.ModePerm); err == nil {
				var localFile *os.File
				if localFile, err = os.OpenFile(saveTo, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666); err == nil {
					var remoteFile os.FileInfo
					if remoteFile, err = client.Stat(ftpFilePath); err == nil {
						progressBar := pb.Full.Start64(remoteFile.Size())
						progressBar.Set("prefix", fmt.Sprintf("[%s] \033[97mUpdating\033[0m \033[92m%s\033[0m", sync.Now(), filepath.Base(saveTo)))
						err = client.Retrieve(ftpFilePath, progressBar.NewProxyWriter(localFile), 3,
							func(offset int64) {
								progressBar.SetCurrent(offset)
							})
						progressBar.Finish()
						if err == nil {
							break
						} else {
							fmt.Printf("[%s] Retrieve \033[97m%s\033[0m \033[91mfailed\033[0m\n", sync.Now(), ftpFilePath)
						}
					} else {
						fmt.Printf("[%s] Get \033[97m%s\033[0m info \033[91mfailed\033[0m\n", sync.Now(), ftpFilePath)
					}
					localFile.Close()
				} else {
					fmt.Printf("[%s] Create \033[97m%s\033[0m \033[91mfailed\033[0m\n", sync.Now(), saveTo)
				}
			} else {
				fmt.Printf("[%s] Create \033[97m%s\033[0m \033[91mfailed\033[0m\n", sync.Now(), filepath.Dir(saveTo))
			}
			client.Close()
		} else {
			fmt.Printf("[%s] Download file from \033[97m%s\033[0m \033[91mfailed\033[0m\n", sync.Now(), sync.Host)
		}
		fmt.Printf("[%s] Wait %d seconds...\n", sync.Now(), WAIT_SECONDS)
		time.Sleep(WAIT_SECONDS * time.Second)
	}
	return
}

//goland:noinspection GoUnhandledErrorResult,SpellCheckingInspection
func (sync FTP_SYNC) wget(ftpFilePath string, saveTo string) (err error) {
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		wget := strings.ReplaceAll(path.Join(exeDir, "wget.exe"), "/", string(os.PathSeparator))
		if _, err := os.Stat(wget); err == nil {
			ready := true
			if _, err := os.Stat(saveTo); err == nil {
				if os.RemoveAll(saveTo) != nil {
					ready = false
				}
			}
			if ready {
				if err := os.MkdirAll(filepath.Dir(saveTo), os.ModePerm); err == nil {
					taskkill := exec.Command("taskkill.exe", "/f", "/im:wget.exe")
					taskkill.Stdout = os.Stdout
					taskkill.Run()
					ftpPath := strings.ReplaceAll(ftpFilePath, string(os.PathSeparator), "/")
					args := "-d "
					args += fmt.Sprintf("-c --tries=%d ", MAX_TRY)
					args += fmt.Sprintf("-O \"%s\" ", saveTo)
					args += fmt.Sprintf("\"ftp://%s:%s@%s%s\" ", sync.UserName, sync.Password, sync.Host, ftpPath)
					if batchFile, err := os.Create("wget.cmd"); err == nil {
						if _, err := batchFile.WriteString(fmt.Sprintf("\"%s\" %s", wget, args)); err == nil {
							ctx, cancel := context.WithTimeout(context.Background(), WGET_TIMEOUT)
							defer cancel()
							cmd := exec.CommandContext(ctx, "cmd.exe", "/c", "wget.cmd")
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							if err := cmd.Run(); err != nil {
								if errors.Is(ctx.Err(), context.DeadlineExceeded) {
									fmt.Printf("[%s] Download %s is \033[91mtimeout\033[0m\n", sync.Now(), ftpPath)
								}
							}
						}
						batchFile.Close()
					}
				}
			}
		}
	}
	return
}

func (sync FTP_SYNC) Now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

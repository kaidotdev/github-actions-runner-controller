package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"regexp"
	"syscall"

	expect "github.com/google/goexpect"
)

type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func check() {
	if _, err := exec.LookPath("bash"); err != nil {
		log.Fatal(err)
	}
}

func install(runnerVersion string) {
	request, err := http.NewRequest("GET", fmt.Sprintf("https://github.com/actions/runner/releases/download/v%s/actions-runner-linux-x64-%s.tar.gz", runnerVersion, runnerVersion), nil)
	if err != nil {
		log.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Fatal(err)
	}

	defer response.Body.Close()
	gzipReader, err := gzip.NewReader(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		if hdr.Typeflag != tar.TypeDir {
			if err := os.MkdirAll(path.Dir(hdr.Name), 0777); err != nil {
				log.Fatal(err)
			}
			f, err := os.Create(hdr.Name)
			if err != nil {
				log.Fatal(err)
			}
			func(f *os.File) {
				defer f.Close()

				if _, err := io.Copy(f, tarReader); err != nil {
					log.Fatal(err)
				}
			}(f)
			if err := os.Chmod(hdr.Name, os.FileMode(hdr.Mode)); err != nil {
				log.Fatal(err)
			}
		}
	}

	command := exec.Command("bash", "bin/installdependencies.sh")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		log.Fatal(err)
	}
}

func getRegistrationToken(repository string, token string) string {
	request, err := http.NewRequest("POST", fmt.Sprintf("https://api.github.com/repos/%s/actions/runners/registration-token", repository), nil)
	if err != nil {
		log.Fatal(err)
	}
	request.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	var registrationTokenResponse TokenResponse
	if err := json.Unmarshal(body, &registrationTokenResponse); err != nil {
		log.Fatal(err)
	}

	return registrationTokenResponse.Token
}

func getRemoveToken(repository string, token string) string {
	request, err := http.NewRequest("POST", fmt.Sprintf("https://api.github.com/repos/%s/actions/runners/remove-token", repository), nil)
	if err != nil {
		log.Fatal(err)
	}
	request.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	var removeTokenResponse TokenResponse
	if err := json.Unmarshal(body, &removeTokenResponse); err != nil {
		log.Fatal(err)
	}

	return removeTokenResponse.Token
}

func run(registrationToken string, repository string, hostname string) {
	e, _, err := expect.Spawn(fmt.Sprintf("bash config.sh --token %s --url https://github.com/%s", registrationToken, repository), -1)
	if err != nil {
		log.Fatal(err)
	}
	_, _, err = e.Expect(regexp.MustCompile("Enter the name of the runner group to add this runner to:"), -1)
	if err != nil {
		log.Fatal(err)
	}
	if err := e.Send("\n"); err != nil {
		log.Fatal(err)
	}
	_, _, err = e.Expect(regexp.MustCompile("Enter the name of runner:"), -1)
	if err != nil {
		log.Fatal(err)
	}
	if err := e.Send(hostname + "\n"); err != nil {
		log.Fatal(err)
	}
	_, _, err = e.Expect(regexp.MustCompile(`Enter any additional labels`), -1)
	if err != nil {
		log.Fatal(err)
	}
	if err := e.Send("\n"); err != nil {
		log.Fatal(err)
	}
	_, _, err = e.Expect(regexp.MustCompile("Enter name of work folder:"), -1)
	if err != nil {
		log.Fatal(err)
	}
	if err := e.Send("\n"); err != nil {
		log.Fatal(err)
	}
	_, _, err = e.Expect(regexp.MustCompile("Settings Saved."), -1)
	if err != nil {
		log.Fatal(err)
	}
	if err := e.Send("exit\n"); err != nil {
		log.Fatal(err)
	}
	command := exec.Command("bash", "run.sh")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		log.Print(err)
	}
}

func remove(registrationToken string) {
	command := exec.Command("bash", "config.sh", "remove", "--token", registrationToken)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		log.Print(err)
	}
}

func main() {
	var runnerVersion string
	var repository string
	var token string
	var hostname string
	var onlyInstall bool
	var withoutInstall bool
	flag.StringVar(&runnerVersion, "runner-version", "2.291.1", "Version of GitHub Actions runner")
	flag.StringVar(&repository, "repository", "kaidotdev/github-actions-runner-controller", "GitHub Repository Name")
	flag.StringVar(&token, "token", "********", "GitHub Token")
	flag.StringVar(&hostname, "hostname", "runner", "Hostname used as Runner name")
	flag.BoolVar(&onlyInstall, "only-install", false, "Execute install only")
	flag.BoolVar(&withoutInstall, "without-install", false, "Execute without install")
	flag.Parse()

	check()
	if !withoutInstall {
		install(runnerVersion)
		if onlyInstall {
			os.Exit(0)
		}
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM)

	log.Printf("Run: %s", hostname)
	registrationToken := getRegistrationToken(repository, token)
	go run(registrationToken, repository, hostname)

	<-quit
	log.Printf("Remove: %s", hostname)
	removeToken := getRemoveToken(repository, token)
	remove(removeToken)
}

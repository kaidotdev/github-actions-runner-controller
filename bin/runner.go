package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	expect "github.com/google/goexpect"
	"golang.org/x/xerrors"
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
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		log.Fatalf("failed to get registration token: %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
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
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		log.Fatalf("failed to get remove token: %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	var removeTokenResponse TokenResponse
	if err := json.Unmarshal(body, &removeTokenResponse); err != nil {
		log.Fatal(err)
	}

	return removeTokenResponse.Token
}

func run(registrationToken string, repository string, hostname string, disableupdate bool) {
	var args []string
	if disableupdate {
		args = append(args, "--disableupdate")
	}
	e, _, err := expect.Spawn(fmt.Sprintf("bash config.sh --labels kaidotdev/github-actions-runner-controller --token %s --url https://github.com/%s %s", registrationToken, repository, strings.Join(args, " ")), -1, expect.Verbose(true), expect.Tee(os.Stdout))
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
		log.Printf("%+v", err)
	}
}

func remove(registrationToken string) {
	command := exec.Command("bash", "config.sh", "remove", "--token", registrationToken)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		log.Printf("%+v", err)
	}
}

func main() {
	var runnerVersion string
	var repository string
	var hostname string
	var token string
	var githubAppId string
	var githubAppInstallationId string
	var githubAppPrivateKey string
	var onlyInstall bool
	var withoutInstall bool
	var disableupdate bool
	flag.StringVar(&runnerVersion, "runner-version", "2.291.1", "Version of GitHub Actions runner")
	flag.StringVar(&repository, "repository", "kaidotdev/github-actions-runner-controller", "GitHub Repository Name")
	flag.StringVar(&token, "token", "********", "GitHub Token")
	flag.StringVar(&hostname, "hostname", "runner", "Hostname used as Runner name")
	flag.StringVar(&token, "token", "********", "GitHub Token")
	flag.StringVar(&githubAppId, "github-app-id", "", "GitHub App ID")
	flag.StringVar(&githubAppInstallationId, "github-app-installation-id", "", "GitHub App Installation ID")
	flag.StringVar(&githubAppPrivateKey, "github-app-private-key", "", "GitHub App Private Key")
	flag.BoolVar(&onlyInstall, "only-install", false, "Execute install only")
	flag.BoolVar(&withoutInstall, "without-install", false, "Execute without install")
	flag.BoolVar(&disableupdate, "disableupdate", false, "Disable self-hosted runner automatic update to the latest released version")
	flag.Parse()

	check()
	if !withoutInstall {
		install(runnerVersion)
		if onlyInstall {
			os.Exit(0)
		}
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGKILL)

	if githubAppId != "" && githubAppInstallationId != "" && githubAppPrivateKey != "" {
		err, jwtToken := signJwt(githubAppPrivateKey, githubAppId)
		if err != nil {
			log.Fatalf("failed to sign jwt: %+v", err)
		}

		accessTokenRequest, err := http.NewRequest("POST", fmt.Sprintf("https://api.github.com/app/installations/%s/access_tokens", githubAppInstallationId), nil)
		if err != nil {
			log.Fatalf("failed to create request: %+v", err)
		}

		accessTokenRequest.Header.Set("Accept", "application/vnd.github+json")
		accessTokenRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *jwtToken))
		accessTokenRequest.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		accessTokenResponse, err := http.DefaultClient.Do(accessTokenRequest)
		if err != nil {
			log.Fatalf("failed to do request: %+v", err)
		}
		defer accessTokenResponse.Body.Close()

		if accessTokenResponse.StatusCode != http.StatusCreated {
			log.Fatalf("failed to get access token: %d", accessTokenResponse.StatusCode)
		}

		accessToken := struct {
			Token string `json:"token"`
		}{}
		if err := json.NewDecoder(accessTokenResponse.Body).Decode(&accessToken); err != nil {
			log.Fatalf("failed to decode access token: %+v", err)
		}

		token = accessToken.Token
	}

	log.Printf("Run: %s", hostname)
	registrationToken := getRegistrationToken(repository, token)
	go run(registrationToken, repository, hostname, disableupdate)

	<-quit
	log.Printf("Remove: %s", hostname)
	removeToken := getRemoveToken(repository, token)
	remove(removeToken)
}

func signJwt(privateKey string, clientId string) (error, *string) {
	block, _ := pem.Decode([]byte(privateKey))
	if block == nil {
		return xerrors.New("failed to decode private key"), nil
	}

	rsaPrivateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return xerrors.Errorf("failed to parse private key: %w", err), nil
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(time.Minute * 10).Unix(),
		"iss": clientId,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	jwtToken, err := token.SignedString(rsaPrivateKey)
	if err != nil {
		return xerrors.Errorf("failed to sign token: %w", err), nil
	}
	return nil, &jwtToken
}

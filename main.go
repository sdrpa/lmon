// https://gobyexample.com/collection-functions
// https://github.com/golang/go/wiki/SliceTricks
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Configuration struct
type Configuration struct {
	NodeURL          string
	PublicKey        string
	Password         string // Forging password, not your wallet password
	Delegate         string
	HomePath         string
	InstallationPath string
	PublicNodeURL    string
}

var config Configuration

// --- Library

// RetryFunc - https://github.com/matryer/try
type RetryFunc func(attempt int) (retry bool, err error)

// Do keeps trying the function until the second argument
// returns false, or no error is returned.
func Do(fn RetryFunc) error {
	maxRetries := 50
	var err error
	var cont bool
	attempt := 1
	for {
		cont, err = fn(attempt)
		if !cont || err == nil {
			break
		}
		attempt++
		if attempt > maxRetries {
			return errors.New("Exceeded max retry limit")
		}
	}
	return err
}

// --- Functions

func isForging() bool {
	url := config.NodeURL + "/api/node/status/forging"
	res, err := http.Get(url)
	if err != nil {
		fmt.Println("Error: Get in isForging")
		return false
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println("Error: ReadAll in isForging")
		return false
	}
	type Node struct {
		Forging bool `json:"forging"`
	}
	type JSON struct {
		Data []Node `json:"data"`
	}
	jsonRes := new(JSON)
	unmarshalErr := json.Unmarshal(body, &jsonRes)
	if unmarshalErr != nil {
		fmt.Println("Error: Unmarshal in isForging")
		return false
	}
	return jsonRes.Data[0].Forging
}

func localVersion() string {
	filePath := config.InstallationPath + "/package.json"
	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		panic(err.Error())
	}
	type Package struct {
		Version string `json:"version"`
	}
	data := Package{}
	_ = json.Unmarshal([]byte(file), &data)
	return data.Version
}

func latestVersion() string {
	url := "https://downloads.lisk.io/lisk/test/latest.txt"
	resp, err := http.Get(url)
	if err != nil {
		panic(err.Error())
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err.Error())
	}
	return strings.TrimSpace(string(data))
}

func download(url string, savePath string) error {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Create the file
	out, err := os.Create(savePath)
	if err != nil {
		return err
	}
	defer out.Close()
	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func update() {
	scriptPath := config.HomePath + "/installLisk.sh"
	os.Remove(scriptPath) // ignore error
	downloadErr := download("https://downloads.lisk.io/lisk/test/installLisk.sh", scriptPath)
	if downloadErr != nil {
		panic(downloadErr.Error)
	}
	chmodErr := os.Chmod(scriptPath, 0755)
	if chmodErr != nil {
		panic(chmodErr.Error)
	}
	fmt.Println("Beginning Lisk update...")
	app := scriptPath
	arg0 := "upgrade"
	arg1 := "-r"
	arg2 := "test"
	arg3 := "-d"
	arg4 := config.HomePath
	arg5 := "-0"
	arg6 := "no"
	cmd := exec.Command(app, arg0, arg1, arg2, arg3, arg4, arg5, arg6)
	stdout, err := cmd.Output()
	if err != nil {
		panic(err.Error())
	}
	fmt.Println(string(stdout))
}

// Peer is a representation of a peer
type Peer struct {
	IP     string `json:"ip"`
	Height int    `json:"height"`
}

func publicPeers() []Peer {
	url := config.PublicNodeURL + ":7000/api/peers?limit=100"
	res, err := http.Get(url)
	if err != nil {
		panic(err.Error())
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err.Error())
	}

	type JSON struct {
		Data []Peer `json:"data"`
	}
	jsonRes := new(JSON)
	unmarshalErr := json.Unmarshal(body, &jsonRes)
	if unmarshalErr != nil {
		panic(unmarshalErr.Error())
	}
	return jsonRes.Data
}

func publicHeight(ps []Peer) int {
	sort.Slice(ps, func(i, j int) bool {
		return ps[i].Height < ps[j].Height
	})
	return ps[0].Height
}

func localHeight() int {
	res, err := http.Get(config.NodeURL + "/api/node/status")
	if err != nil {
		panic(err.Error())
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err.Error())
	}

	type JSON struct {
		Data Peer `json:"data"`
	}
	jsonRes := new(JSON)
	unmarshalErr := json.Unmarshal(body, &jsonRes)
	if unmarshalErr != nil {
		panic(unmarshalErr.Error())
	}

	return jsonRes.Data.Height
}

func isAPIReady() (string, error) {
	resp, err := http.Get(config.NodeURL + "/api/node/status")
	if err != nil {
		return "", err
	}
	return resp.Status, nil
}

func reload() {
	app := config.InstallationPath + "/lisk.sh"
	arg0 := "reload"
	cmd := exec.Command(app, arg0)
	stdout, err := cmd.Output()
	if err != nil {
		panic(err.Error())
	}
	fmt.Println(string(stdout))
}

func waitUntilAPIReady() {
	// Node API isn't available immediately after the restart.
	// Wait API to be ready.
	doErr := Do(func(attempt int) (bool, error) {
		const (
			maxAttempts                   = 10
			delayBetweenAttemptsInSeconds = 10
		)
		_, err := isAPIReady()
		if err != nil {
			time.Sleep(time.Second * delayBetweenAttemptsInSeconds)
		}
		return attempt < maxAttempts, err
	})
	if doErr != nil {
		panic(doErr.Error())
	}
	fmt.Println("Node API is ready")
}

func enableForging() {
	url := config.NodeURL + "/api/node/status/forging"
	jsonString := "{\"forging\": true, \"publicKey\": \"" + config.PublicKey + "\", \"password\": \"" + config.Password + "\"}"
	payload := []byte(jsonString)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()

	// Get enable forging request response
	body, _ := ioutil.ReadAll(res.Body)
	type Node struct {
		Forging bool `json:"forging"`
	}
	type JSON struct {
		Data []Node `json:"data"`
	}
	jsonRes := new(JSON)
	unmarshalErr := json.Unmarshal(body, &jsonRes)
	if unmarshalErr != nil {
		panic(unmarshalErr)
	}
	if jsonRes.Data[0].Forging != true {
		panic("Could not enable forging")
	}
	fmt.Println("Forging is enabled")
}

func missedBlocks() int {
	res, err := http.Get(config.NodeURL + "/api/delegates?username=" + config.Delegate)
	if err != nil {
		panic(err.Error())
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err.Error())
	}
	type Node struct {
		MissedBlocks int `json:"missedBlocks"`
	}
	type JSON struct {
		Data []Node `json:"data"`
	}
	jsonRes := new(JSON)
	unmarshalErr := json.Unmarshal(body, &jsonRes)
	if unmarshalErr != nil {
		panic(unmarshalErr)
	}
	return jsonRes.Data[0].MissedBlocks
}

// ---
// Auto-update and enable forging
// Reload if number of missed blocks has increased from last check
// Reload if localHeight+1 < publicHeight

func needsUpdate() bool {
	return localVersion() != latestVersion()
}

func needsReload(prevMissedBlocks int) bool {
	return localHeight()+1 < publicHeight(publicPeers()) ||
		missedBlocks() > prevMissedBlocks
}

func loadConfiguration(filename string) Configuration {
	file, _ := os.Open(filename)
	defer file.Close()
	decoder := json.NewDecoder(file)
	config := Configuration{}
	err := decoder.Decode(&config)
	if err != nil {
		panic(err.Error())
	}
	return config
}

func main() {
	config = loadConfiguration("config.json")
	fmt.Println(config)

	// Check whether node is running
	_, err := http.Get(config.NodeURL + "/api/node/status")
	if err != nil {
		panic("Lisk must be running before running the script")
	}

	const interval = 5 // check interval in seconds
	prevMissedBlocks := missedBlocks()
	// Infinite loop ...
	for {
		if needsUpdate() {
			update()
			waitUntilAPIReady()
			enableForging()
		}
		if needsReload(prevMissedBlocks) {
			reload()
			waitUntilAPIReady()
			enableForging()
		}
		prevMissedBlocks = missedBlocks()
		time.Sleep(time.Second * interval)

		fmt.Println("Done", time.Now().String())
	}
}

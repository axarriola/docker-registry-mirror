package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// structs used to unmarshall configuration file
type ConfigFile struct {
	Src  Config
	Dest Config
}

type Config struct {
	Host      string
	User      string
	Pass      string
	Transport string
	Ssl       bool
	Api       string
}

// registry v2/_catalog api call response
type RegistryCatalog struct {
	Repositories []string `json:"repositories"`
}

var (
	srcConfig     Config
	destConfig    Config
	skopeoSyncCmd string
)

// registry.conf text templates
var (
	insecureRegistryTemplate1 = "[registries.insecure]\nregistries = ['{{ .Host }}']\n"
	insecureRegistryTemplate2 = "[registries.insecure]\nregistries = ['{{ .Src.Host }}', '{{ .Dest.Host }}']\n"
)

func main() {
	err := readConfig()
	if err != nil {
		log.Fatalf("ERROR: Unable to read Config %v", err)
	}

	err = prepareSkopeoSyncCmd()
	if err != nil {
		log.Fatalf("ERROR: Unable to prepare skopeo sync command %v", err)
	}

	interv, exists := os.LookupEnv("INTERVAL")
	if !exists {
		interv = "86400"
	}

	interval, err := strconv.Atoi(interv)
	if err != nil {
		log.Fatalf("ERROR: Unable to convert interval %v to int %v", interv, err)
	}

	// infinite loop
	// - Get repository list from src registry (v2/_catalog)
	// - call syncRepo() for every registry
	// - sleep for INTERVAL seconds
	for {
		repos, err := getRepoList(&srcConfig)
		log.Printf("Following repositories will be synced:\n%v", repos)
		if err != nil {
			log.Fatalf("ERROR: Unable to get source repo list %v", err)
		}

		for _, r := range repos {
			log.Printf("Syncing repository %s", r)
			err = syncRepo(r)
			if err != nil {
				log.Printf("ERROR: unable to sync repo %s %v\n", r, err)
			}
		}

		log.Printf("Finished, sleeping for %d seconds.", interval)
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func getRepoList(regConfig *Config) ([]string, error) {
	// use http if ssl is set to false in config, otherwise use https
	proto := "https"
	if !regConfig.Ssl {
		proto = "http"
	}

	// use host from the config for the v2/_catalog api call
	// if api is set in config, use that one instead
	url := fmt.Sprintf("%s://%s/v2/_catalog", proto, regConfig.Host)
	if regConfig.Api != "" {
		url = fmt.Sprintf("%s://%s/v2/_catalog", proto, regConfig.Api)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error creating %s request: %v", proto, err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error executing %s request: %v", proto, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("HTTP response status: %s", resp.Status))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error reading response body: %v", err))
	}

	var temp RegistryCatalog
	err = json.Unmarshal(body, &temp)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error in json unmarshal: %v", err))
	}

	return temp.Repositories, nil
}

func syncRepo(repo string) error {
	// To keep the same repository names (folders) its necessary to set the whole repository
	// path, except the image name in the "skopeo sync" destination registry/repo argument
	split := strings.Split(repo, "/")

	var destRepo string
	if len(split) == 1 {
		destRepo = repo
	} else {
		destRepo = strings.Join(split[:len(split)-1], "/")
	}

	cmd := fmt.Sprintf("%s %s/%s %s/%s", skopeoSyncCmd, srcConfig.Host, repo, destConfig.Host, destRepo)

	err := execCmdSh(cmd)
	if err != nil {
		return err
	}

	return nil
}

func prepareSkopeoSyncCmd() error {
	cmd := "skopeo sync"

	// if any of the two registries has "ssl: false" in config
	// add the registry.conf as parameter to "skopeo sync"
	// registry.conf is created by createSkopeoConfig{1,2}()
	// and it holds the insecure registries list
	if !(srcConfig.Ssl && destConfig.Ssl) {
		if _, err := os.Stat("registry.conf"); err != nil {
			return errors.New(fmt.Sprintf("File registry.conf not present"))
		}
		cmd = fmt.Sprintf("%s --registries-conf=registry.conf", cmd)
	}

	// set credential arguments if not empty
	var cred string
	if srcConfig.User != "" {
		cred = fmt.Sprintf("--src-creds %s", srcConfig.User)
		if srcConfig.Pass != "" {
			cred = fmt.Sprintf("%s:%s", cred, srcConfig.Pass)
		}
	}

	if destConfig.User != "" {
		cred = fmt.Sprintf("%s --dest-creds %s", cred, destConfig.User)
		if destConfig.Pass != "" {
			cred = fmt.Sprintf("%s:%s", cred, destConfig.Pass)
		}
	}

	cmd = fmt.Sprintf("%s %s", cmd, cred)

	// TODO: Add certs path flag (currently I attach the ca rootchain in the container)
	if !srcConfig.Ssl {
		cmd = fmt.Sprintf("%s --src-tls-verify=false", cmd)
	}
	if !destConfig.Ssl {
		cmd = fmt.Sprintf("%s --dest-tls-verify=false", cmd)
	}

	cmd = fmt.Sprintf("%s --src %v --dest %v", cmd, srcConfig.Transport, destConfig.Transport)

	skopeoSyncCmd = cmd
	return nil
}

func execCmdSh(cmdstr string) error {
	// execute cmdstr with exec.Command().Run()
	// print Stdout and if error return stderr
	c := exec.Command("sh", "-c", cmdstr)

	c.Stdin = strings.NewReader("")

	var out, outerr bytes.Buffer
	c.Stdout = &out
	c.Stderr = &outerr

	err := c.Run()
	if err != nil {
		return errors.New(fmt.Sprintf("Error while executing command:\n'%s'\nError: %v\n%s", cmdstr, err, outerr.String()))
	}

	log.Println(out.String())
	return nil
}

func readConfig() error {
	// config.yml is mounted to /config.yml in docker-compose file.
	// Location can be changed by setting the CONFIG env var,
	// docker-compose file should be updated in that case.
	configFile, exists := os.LookupEnv("CONFIG")
	if !exists {
		configFile = "./config.yml"
	}

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return errors.New(fmt.Sprintf("Error reading file %s: %v", configFile, err))
	}

	var tmpc ConfigFile
	err = yaml.Unmarshal(data, &tmpc)
	if err != nil {
		return fmt.Errorf("error parsing config file '%s': %v", configFile, err)
	}

	srcConfig = tmpc.Src
	destConfig = tmpc.Dest

	// if transport is not defined, use docker as default
	if srcConfig.Transport == "" {
		srcConfig.Transport = "docker"
	}

	if destConfig.Transport == "" {
		destConfig.Transport = "docker"
	}

	// if one of the two registries has "ssl: false" a registry.conf file has to be
	// created with the insecure registries list.
	// the 3 cases are: both are insecure, src is insecure, dest is insecure.
	if !srcConfig.Ssl && destConfig.Ssl {
		err = createSkopeoConfig1(&srcConfig)
		if err != nil {
			return errors.New(fmt.Sprintf("error creating Skopeo registries config: %v", err))
		}
	} else if srcConfig.Ssl && !destConfig.Ssl {
		err = createSkopeoConfig1(&destConfig)
		if err != nil {
			return errors.New(fmt.Sprintf("error creating Skopeo registries config: %v", err))
		}
	} else if !srcConfig.Ssl && !destConfig.Ssl {
		err = createSkopeoConfig2(ConfigFile{Src: srcConfig, Dest: destConfig})
		if err != nil {
			return errors.New(fmt.Sprintf("error creating Skopeo registries config: %v", err))
		}
	}

	return nil
}

// This two functions differ only by type of the parameter received and template used.
// The first one covers the case of one insecure registry, the second one both insecure
func createSkopeoConfig1(c *Config) error {
	tmpl, err := template.New("conf").Parse(insecureRegistryTemplate1)
	if err != nil {
		return err
	}

	filename := "./registry.conf"
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	err = tmpl.Execute(f, c)
	if err != nil {
		return err
	}
	return nil
}

func createSkopeoConfig2(c ConfigFile) error {
	tmpl, err := template.New("conf").Parse(insecureRegistryTemplate2)
	if err != nil {
		return err
	}

	filename := "./registry.conf"
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	err = tmpl.Execute(f, c)
	if err != nil {
		return err
	}
	return nil
}

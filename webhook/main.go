package main

import (
	"crypto/sha256"
	"flag"
	"io/ioutil"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"
)

func main() {
	var parameters WhSvrParameters

	// get CLI Parameters
	flag.IntVar(&parameters.port, "port", 8080, "Webhook Server Port")
	flag.StringVar(&parameters.certFile, "tlsCertFile", "/etc/webhook/certs/cert.pem", "x509 Certificate File for HTTPS")
	flag.StringVar(&parameters.keyFile, "tlsKeyFile", "/etc/webhook/certs/key.pem", "x509 Certificate Private Key")
	flag.StringVar(&parameters.initConainerConfigFile, "initContainerConfig", "/etc/webhook/config/init-container.yaml", "Mutation Configuration File")

	// read init container configuration
	initContainerConfig, err := loadConfig(parameters.initConainerConfigFile)
	if err != nil {
		glog.Exit("Failed to Load Init Container Configuration.")
	}
}

func loadConfig(configFilePath string) (*Config, error) {
	data, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		glog.Errorf("Failed to Load Init Container Configuration : %s", err)
		return nil, err
	}

	glog.Infof("New configuration: sha256sum %x", sha256.Sum256(data))

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		glog.Errorf("Failed to Read Init Container Configuration : %s", err)
		return nil, err
	}

	return &config, nil
}

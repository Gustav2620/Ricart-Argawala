package main

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"
)

type NodeConfig struct {
	ID   int    `yaml:"id"`
	Addr string `yaml:"addr"`
}

type Config struct {
	Nodes []NodeConfig `yaml:"nodes"`
}

func LoadConfig(filename string) (Config, error) {
	var cfg Config
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return cfg, err
	}
	err = yaml.Unmarshal(data, &cfg)
	return cfg, err
}

func BuildNodeMap(myID int, cfg Config) (myAddr string, peers map[int]string) {
	peers = make(map[int]string)
	for _, n := range cfg.Nodes {
		if n.ID == myID {
			myAddr = n.Addr
		} else {
			peers[n.ID] = n.Addr
		}
	}
	if myAddr == "" {
		log.Fatalf("Node ID %d not found in config.yaml", myID)
	}
	return myAddr, peers
}
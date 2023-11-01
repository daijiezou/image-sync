package config

import (
	"github.com/spf13/viper"
	"log"
)

type GlobalConfig struct {
	SourceRegistryAddr string
	TargetRegistryAddr string
	OutputPath         string
	ImageListPath      string
	StartTime          string
	EndTime            string
	DbDsn              string
	Proc               int
	SyncMethod         string
	SystemUsername     string
	SystemPassword     string
}

var IMConfig *GlobalConfig

func ParseConfig(projectName, configFile string) {
	viper.SetConfigFile(configFile)
	err := viper.ReadInConfig()
	if err != nil {
		log.Println("ParseConfigError:", err)
		panic(err)
	}
	IMConfig = new(GlobalConfig)
	err = viper.UnmarshalKey(projectName, IMConfig)
	if err != nil {
		log.Println("UnmarshalConfigError:", err)
		panic(err)
	}
}

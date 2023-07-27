package config

import (
	"github.com/spf13/viper"
)


type GlobalConfig struct {
	RemoteRegistry   string
	RegistryUsername string
	RegistryPassword string
	Namespace        string

	DbDsn                 string
	LocalRegistry         string
	LocalRegistryUsername string
	LocalRegistryPassword string

	StartImageId int
	EndImageId   int
}

var Config *GlobalConfig

func ParseConfig(projectName, configFile string) {
	viper.SetConfigFile(configFile)
	err := viper.ReadInConfig()
	if err != nil {

		panic(err)
	}
	Config = new(GlobalConfig)
	err = viper.UnmarshalKey(projectName, Config)
	if err != nil {
		panic(err)
	}
}

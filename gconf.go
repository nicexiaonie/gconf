package gconf

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type Gconf struct {
}
type Config struct {
	ConfigPath         string
	ConfigName         string
	WatchConfig        bool
	CallOnConfigChange func(in fsnotify.Event)
}

func New(c Config) (*viper.Viper, error) {
	v := viper.New()

	v.AddConfigPath(c.ConfigPath)
	v.SetConfigName(c.ConfigName)
	err := v.ReadInConfig()
	if err != nil {
		return v, err
	}

	if c.WatchConfig {
		v.WatchConfig()
		v.OnConfigChange(func(in fsnotify.Event) {
			fmt.Printf("权限配置监控变化: Name:%s, Op:%s, String:%s  \n", in.Name, in.Op, in.String())

		})
	}
	v.OnConfigChange(c.CallOnConfigChange)
	return v, nil
}

package main

import (
	"fmt"
	"github.com/nicexiaonie/gconf"
)

func main()  {

	c := gconf.Config{
		ConfigPath : "./",
		ConfigName : "test",
	}

	gc,_ := gconf.New(c)

	fmt.Println(gc.Get("aa"))


}

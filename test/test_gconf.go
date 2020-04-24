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


	gc.Set("aa", "cc")

	gc.WriteConfig()



	gc.RegisterAlias("loud", "Verbose")
	gc.Set("loud", "aaa")


	fmt.Println(gc.Get("Verbose"))




	gc.AutomaticEnv()

	fmt.Println(gc.Get("PATH"))

	fmt.Println(gc.Get("aa"))


}

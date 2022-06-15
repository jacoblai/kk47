package main

import (
	"container/list"
	"context"
	"flag"
	"fmt"
	"github.com/bytedance/sonic/decoder"
	"github.com/jacoblai/httprouter"
	"github.com/libp2p/go-reuseport"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"reuserhttp/cors"
	"reuserhttp/resultor"
	"runtime"
	"syscall"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
	timeLocation, _ := time.LoadLocation("Asia/Shanghai") //使用时区码
	time.Local = timeLocation
}

func setLimit() {
	// Increase resources limitations
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Fatal(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Fatal(err)
	}
}

func main() {
	setLimit()
	var (
		addr = flag.String("l", ":7003", "绑定Host地址")
	)
	flag.Parse()

	queue := list.New()

	router := httprouter.New()
	router.POST("/", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		var obj map[string]interface{}
		dc := decoder.NewStreamDecoder(r.Body)
		dc.UseInt64()
		err := dc.Decode(&obj)
		if err != nil {
			resultor.RetErr(w, err)
			return
		}
		queue.PushBack(obj)
		resultor.RetOk(w, obj, 1)
	})
	router.GET("/", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		item := queue.Front()
		if item == nil {
			resultor.RetErr(w, "wow")
			return
		}
		queue.Remove(item)
		resultor.RetOk(w, item, 1)
	})

	srv := &http.Server{Handler: cors.CORS(router), ErrorLog: nil}
	srv.Addr = *addr
	go func() {
		for i := 0; i < runtime.NumCPU(); i++ {
			ln, err := reuseport.Listen("tcp", *addr)
			if err != nil {
				log.Fatal(err)
			}
			if err := srv.Serve(ln); err != nil {
				_ = ln.Close()
			}
		}
	}()
	fmt.Printf("Server with is running on prot [%s] \n", srv.Addr)

	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	cleanup := make(chan bool)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range signalChan {
			ctx, _ := context.WithTimeout(context.Background(), 60*time.Second)
			go func() {
				_ = srv.Shutdown(ctx)
				cleanup <- true
			}()
			<-cleanup
			fmt.Println("safe exit")
			cleanupDone <- true
		}
	}()
	<-cleanupDone
}

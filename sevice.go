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
	"sync"
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
	//dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	//if err != nil {
	//	log.Println(err)
	//	return
	//}

	queue := list.New()
	lc := &sync.RWMutex{}
	wg := &sync.WaitGroup{}

	router := httprouter.New()
	router.POST("/", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		log.Println(r.Header.Get("custom"))
		defer r.Body.Close()
		wg.Add(1)
		defer wg.Done()
		var obj map[string]interface{}
		dc := decoder.NewStreamDecoder(r.Body)
		dc.UseInt64()
		err := dc.Decode(&obj)
		if err != nil {
			resultor.RetErr(w, err)
			return
		}
		lc.Lock()
		queue.PushBack(obj)
		lc.Unlock()
		resultor.RetOk(w, "ok", 1)
	})
	router.GET("/", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer r.Body.Close()
		lc.RLock()
		item := queue.Front()
		lc.RUnlock()
		if item == nil {
			resultor.RetErr(w, "wow")
			return
		}
		lc.Lock()
		queue.Remove(item)
		lc.Unlock()
		resultor.RetOk(w, item.Value, 1)
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
			fmt.Println("waitting for slow ops....")
			wg.Wait()
			<-cleanup
			fmt.Println("safe exit")
			cleanupDone <- true
		}
	}()
	<-cleanupDone
}

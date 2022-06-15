package resultor

import (
	"bytes"
	"github.com/bytedance/sonic/encoder"
	"net/http"
	"reflect"
	"strings"
	"sync"
)

var pool = &sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func RetChanges(w http.ResponseWriter) {
	_, _ = w.Write([]byte(`{"code":0}`))
}

func RetOk(w http.ResponseWriter, result interface{}, count int) {
	resValue := reflect.ValueOf(result)
	if resValue.IsZero() {
		_, _ = w.Write([]byte(`{"code":0,"count":0}`))
		return
	}
	if resValue.Kind() == reflect.Ptr {
		resValue = resValue.Elem()
	}
	res := make(map[string]interface{})
	res["code"] = 0
	res["data"] = result
	res["count"] = count
	buf := pool.Get().(*bytes.Buffer)
	defer pool.Put(buf)
	buf.Reset()
	enc := encoder.NewStreamEncoder(buf)
	_ = enc.Encode(res)
	_, _ = w.Write(buf.Bytes())
}

func Encode(obj interface{}) []byte {
	buf := pool.Get().(*bytes.Buffer)
	defer pool.Put(buf)
	buf.Reset()
	enc := encoder.NewStreamEncoder(buf)
	_ = enc.Encode(obj)
	return buf.Bytes()
}

func RetErr(w http.ResponseWriter, errmsg interface{}) {
	if v, ok := errmsg.(error); ok {
		errmsg = v.Error()
	}
	emsg := errmsg.(string)
	if strings.HasPrefix(emsg, `"`) {
		_, _ = w.Write([]byte(`{"code":-1,"message":` + emsg + `}`))
		return
	}
	_, _ = w.Write([]byte(`{"code":-1,"message":"` + emsg + `"}`))
}

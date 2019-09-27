
package main

import (
    "bytes"
    "strings"
    "compress/zlib"
    "fmt"
    "io"
)

func main() {

    c := []byte("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")
    kc := []byte("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")

    var in bytes.Buffer
    w,err := zlib.NewWriterLevel(&in,zlib.BestCompression-1)
    if err != nil {
	fmt.Println("compress fail.")
	return
    }
    fmt.Println("==================1111111===========================")

    for i := 0;i<5000;i++ {
	cc := string(c)
	cc = cc + string(kc)
	c = []byte(cc)
    }

    w.Write(c)
    w.Close()

    fmt.Println("==================222222===========================")
    fmt.Println(len(c))
    var s string
    s = in.String()
    //fmt.Println(s)
    fmt.Println(len(s))

    var data bytes.Buffer
    data.Write([]byte(s))

    r,_ := zlib.NewReader(&data)
    var out bytes.Buffer
    io.Copy(&out, r)
    //fmt.Println(out.String())

    if strings.EqualFold(out.String(),string(c)) {
	 fmt.Println("=============================================")
    }
}


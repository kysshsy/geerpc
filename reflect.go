package geerpc

import (
	"fmt"
	"reflect"
)

func main() {
	num := 10
	v := reflect.ValueOf(&num)

	fmt.Println(v)
}

package utils

import (
	"fmt"
	"testing"
)

func TestCopyStructFields(t *testing.T) {
	type Src struct {
		Name string
		Age  int
		Sex  string
	}

	type Dst struct {
		Name string
		Age  int
		Sex  string
	}

	src := Src{
		"Bob",
		20,
		"male",
	}
	dst := Dst{}
	err := CopyStructFields(&src, &dst)
	if err != nil {
		panic(err)
	}
	fmt.Print(dst)
}

func TestMapToStruct(t *testing.T) {
	src := map[string]any{
		"name":      "Ben",
		"nick_name": "BB",
		"age":       false,
	}
	type Dst struct {
		Name     string
		NickName string
		Age      bool
	}
	dst := Dst{}
	err := MapToStruct(src, &dst)
	if err != nil {
		panic(err)
	}
	fmt.Print(dst)
}

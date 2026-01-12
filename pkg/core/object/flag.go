package object

import "fmt"

type SliceFlag []string

func (i *SliceFlag) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *SliceFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func (i *SliceFlag) Items() []string {
	return *i
}

func (i *SliceFlag) Len() int {
	return len(*i)
}

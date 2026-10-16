package main

import (
	desk "example.com/stockdesk"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func run(args []string) error {
	d := desk.NewDesk()
	for _, arg := range args {
		parts := strings.Split(arg, ":")
		number := func(index int) (int, error) {
			if index >= len(parts) {
				return 0, fmt.Errorf("quantity required")
			}
			return strconv.Atoi(parts[index])
		}
		switch parts[0] {
		case "receive", "reopen":
			if len(parts) != 3 {
				return fmt.Errorf("expected operation:id:quantity")
			}
			q, err := number(2)
			if err != nil {
				return err
			}
			if parts[0] == "receive" {
				err = d.Receive(parts[1], q)
			} else {
				err = d.Reopen(parts[1], q)
			}
			if err != nil {
				return err
			}
		case "reserve":
			if len(parts) != 4 {
				return fmt.Errorf("expected reserve:id:sku:quantity")
			}
			q, err := number(3)
			if err != nil {
				return err
			}
			if err = d.Reserve(parts[1], parts[2], q); err != nil {
				return err
			}
		case "dispatch":
			if len(parts) != 4 {
				return fmt.Errorf("expected dispatch:id:request:quantity")
			}
			q, err := number(3)
			if err != nil {
				return err
			}
			if _, err = d.Dispatch(parts[1], parts[2], q); err != nil {
				return err
			}
		case "release":
			if len(parts) != 2 {
				return fmt.Errorf("expected release:id")
			}
			if err := d.Release(parts[1]); err != nil {
				return err
			}
		case "balance":
			if len(parts) != 2 {
				return fmt.Errorf("expected balance:sku")
			}
			fmt.Println(d.Available(parts[1]))
		case "order":
			if len(parts) != 2 {
				return fmt.Errorf("expected order:id")
			}
			row, err := d.Order(parts[1])
			if err != nil {
				return err
			}
			fmt.Printf("remaining=%d shipped=%d cycle=%d\n", row.Remaining, row.Shipped, row.Cycle)
		default:
			return fmt.Errorf("unknown operation: %s", parts[0])
		}
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

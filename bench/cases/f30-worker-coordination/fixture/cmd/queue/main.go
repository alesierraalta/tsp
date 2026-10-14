package main

import (
	"bufio"
	"context"
	"encoding/json"
	q "example.com/workqueue"
	"fmt"
	"os"
	"sync"
	"time"
)

type command struct {
	Op           string
	Name         string
	Key          string
	Owner        string
	Value        string
	Milliseconds int
}

func main() {
	config := q.DefaultConfig()
	config.LeaseTTL = time.Second
	manager, _ := q.NewManager(config)
	now := time.Unix(100, 0)
	leases := make(map[string]q.Lease)
	futures := make(map[string]*q.Future)
	var mu sync.Mutex
	value := "initial"
	held := false
	gate := make(chan struct{})
	started := make(chan struct{}, 1024)
	service := q.NewService(func(key string) (string, error) {
		mu.Lock()
		snapshot, wait, barrier := value, held, gate
		mu.Unlock()
		started <- struct{}{}
		if wait {
			<-barrier
		}
		return snapshot, nil
	})
	rejected := false
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var cmd command
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			panic(err)
		}
		switch cmd.Op {
		case "start":
			lease, ok := manager.Start(cmd.Key, cmd.Owner, now)
			if ok {
				leases[cmd.Name] = lease
			}
		case "advance":
			now = now.Add(time.Duration(cmd.Milliseconds) * time.Millisecond)
		case "finish":
			rejected = manager.Finish(leases[cmd.Name]) != nil
		case "leases":
			fmt.Printf("rejected=%t active=%d completed=%d\n", rejected, manager.Active(), manager.Completions(cmd.Key))
		case "set":
			mu.Lock()
			value = cmd.Value
			mu.Unlock()
		case "hold":
			mu.Lock()
			held = true
			mu.Unlock()
		case "fetch":
			before := service.Stats().Started
			futures[cmd.Name] = service.Fetch(cmd.Key)
			if service.Stats().Started > before {
				<-started
			}
		case "invalidate":
			service.Invalidate(cmd.Key)
		case "release":
			mu.Lock()
			close(gate)
			held = false
			gate = make(chan struct{})
			mu.Unlock()
		case "values":
			first, err := futures["first"].Await(context.Background())
			if err != nil {
				panic(err)
			}
			second, err := futures["second"].Await(context.Background())
			if err != nil {
				panic(err)
			}
			fmt.Printf("first=%s second=%s\n", first, second)
		default:
			panic("unknown operation")
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

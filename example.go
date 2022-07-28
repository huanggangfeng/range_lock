package main

import (
	"log"
	"sync"
	"time"

	"github.com/huanggangfeng/range_lock/lock"
)

func main() {
	log.Println("-------------------------test1--------------------------")
	test1()
	log.Println("-------------------------test2--------------------------")
	test2()
	log.Println("-------------------------test3--------------------------")
	test3()
}

func test1() {

	lk1 := lock.New(1, lock.TypeRead, 30, 100)
	lk2 := lock.New(1, lock.TypeWrite, 1, 50)
	lk3 := lock.New(1, lock.TypeWrite, 0, 30)
	lk4 := lock.New(1, lock.TypeWrite, 20, 100)

	wg.Add(4)
	go runLock("lk1_1", lk1, 0, time.Second*3)
	go runLock("lk1_2", lk2, time.Second, time.Second*2)
	go runLock("lk1_3", lk3, time.Second, time.Second*2)
	go runLock("lk1_4", lk4, time.Second*2, time.Second*2)

	wg.Wait()
	time.Sleep(time.Second * 2)
}

var wg sync.WaitGroup

func test2() {
	lk1 := lock.New(2, lock.TypeWrite, 30, 100)
	lk2 := lock.New(2, lock.TypeRead, 50, 100)
	lk3 := lock.New(2, lock.TypeRead, 0, 50)
	lk4 := lock.New(2, lock.TypeRead, 80, 100)

	wg.Add(4)
	go runLock("lk2_1", lk1, 0, time.Second*2)
	time.Sleep(time.Microsecond)
	go runLock("lk2_2", lk2, time.Second, time.Second*2)
	time.Sleep(time.Microsecond)
	go runLock("lk2_3", lk3, time.Second, time.Second*3)
	time.Sleep(time.Microsecond)
	go runLock("lk2_4", lk4, time.Second, time.Second*4)

	// time.Sleep(time.Second)
	// log.Println("unlock:", "lk1")

	wg.Wait()
	time.Sleep(time.Second * 1)
}

func runLock(name string, lk *lock.Lock, waitTime, holdTime time.Duration) {
	time.Sleep(waitTime)
	defer wg.Done()
	lk.Lock()
	log.Println("--------lock-------:", name)
	time.Sleep(holdTime)
	lk.Unlock()
	log.Println("--------unlock-----:", name)
}

// panic expected
func test3() {
	lk1 := lock.New(3, lock.TypeRead, 30, 100)
	lk2 := lock.New(3, lock.TypeWrite, 50, 100)
	wg.Add(3)
	go runLock("lk3_1", lk1, 0, time.Second*2)
	go runLock("lk3_2", lk2, time.Second, time.Second*1)
	go runLock("lk3_1", lk1, time.Second, time.Second*2)
	wg.Wait()
	time.Sleep(time.Second * 3)
}

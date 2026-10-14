package batch
import (
 "encoding/binary"
 "testing"
)
func TestVisit(t *testing.T) {
 var got []uint64
 VisitIDs([]uint64{4,9}, func(b []byte) { got=append(got,binary.BigEndian.Uint64(b)) })
 if len(got)!=2 || got[0]!=4 || got[1]!=9 { t.Fatal(got) }
}
func TestLabels(t *testing.T) {
 calls:=0
 l:=NewLabels(4,func(id string) string { calls++; return "label:"+id })
 if l.Label("a")!="label:a" || l.Label("a")!="label:a" || calls!=1 { t.Fatal(calls) }
}

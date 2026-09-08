package probe

import "testing"

func TestSelectZorrenEvents(t *testing.T) {
 cases := []struct { name string; in []ZorrenEvent; limit int; want []ZorrenEvent }{
  {"rank_and_preserve", []ZorrenEvent{{"z-low","mote",2,7,"pl"},{"z-high","flit",9,2,"ph"},{"z-mid","dray",5,4,"pm"},{"z-top","zirp",7,3,"pt"}},3,[]ZorrenEvent{{"z-high","flit",9,2,"ph"},{"z-top","zirp",7,3,"pt"},{"z-mid","dray",5,4,"pm"}}},
  {"cutoff_keeps_earlier", []ZorrenEvent{{"z-early","mote",8,10,"pe"},{"z-middle","flit",8,20,"pm"},{"z-late","dray",8,30,"pl"},{"z-tail","zirp",1,40,"pt"}},2,[]ZorrenEvent{{"z-early","mote",8,10,"pe"},{"z-middle","flit",8,20,"pm"}}},
  {"late_higher_weight", []ZorrenEvent{{"z-old","mote",1,1,"po"},{"z-mid","flit",4,2,"pm"},{"z-high","dray",9,3,"ph"}},2,[]ZorrenEvent{{"z-high","dray",9,3,"ph"},{"z-mid","flit",4,2,"pm"}}},
  {"stable_equal_weight", []ZorrenEvent{{"z-late","mote",6,20,"pl"},{"z-early","flit",6,10,"pe"},{"z-low","dray",1,30,"po"}},2,[]ZorrenEvent{{"z-early","flit",6,10,"pe"},{"z-late","mote",6,20,"pl"}}},
  {"empty_input",nil,3,[]ZorrenEvent{}},
  {"non_positive_limit",[]ZorrenEvent{{"z-only","zirp",5,1,"px"}},-1,[]ZorrenEvent{}},
 }
 for _, tc := range cases { t.Run(tc.name,func(t *testing.T) {
  before := append([]ZorrenEvent(nil),tc.in...)
  got := SelectZorrenEvents(tc.in,tc.limit)
  if (got == nil) != (tc.want == nil) { t.Fatalf("nil result = %v, want %v",got == nil,tc.want == nil) }
  if len(got) != len(tc.want) { t.Fatalf("len(result) = %d, want %d",len(got),len(tc.want)) }
  for i := range tc.want { if got[i] != tc.want[i] { t.Fatalf("result[%d] = %#v, want %#v",i,got[i],tc.want[i]) } }
  for i := range before { if tc.in[i] != before[i] { t.Fatalf("input[%d] changed",i) } }
 }) }
}

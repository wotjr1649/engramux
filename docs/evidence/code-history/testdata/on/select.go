package probe

type ZorrenEvent struct {
 ID string
 Kind string
 Weight int
 Sequence int
 Payload string
}

func SelectZorrenEvents(events []ZorrenEvent, limit int) []ZorrenEvent {
 if limit <= 0 { return []ZorrenEvent{} }
 selected := make([]ZorrenEvent, 0, limit)
 for _, event := range events {
  position := 0
  for position < len(selected) {
   current := selected[position]
   if current.Weight > event.Weight || (current.Weight == event.Weight && current.Sequence <= event.Sequence) {
    position++
    continue
   }
   break
  }
  if position >= limit { continue }
  selected = append(selected, ZorrenEvent{})
  copy(selected[position+1:], selected[position:])
  selected[position] = event
  if len(selected) > limit { selected = selected[:limit] }
 }
 return selected
}

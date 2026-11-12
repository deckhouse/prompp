package block

// CreatedWritersCount returns the number of block writers that [Writer.Write] creates for the shard.
func (w *Writer[TShard]) CreatedWritersCount(sd TShard, numberOfShards uint16) (int, error) {
	writers := w.createWriters(sd, numberOfShards)
	return len(writers), writers.Close()
}

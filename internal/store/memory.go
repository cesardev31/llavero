package store

// ApproxMemory devuelve una estimación conservadora del tamaño de claves y
// valores vivos. No incluye toda la sobrecarga interna de mapas/slices de Go.
func (s *Store) ApproxMemory() int64 {
	var total int64
	for _, snap := range s.Snapshot() {
		total += int64(len(snap.Key))
		switch snap.Type {
		case ValueString:
			total += int64(len(snap.Value))
		case ValueList:
			for _, item := range snap.List {
				total += int64(len(item))
			}
		case ValueHash:
			for field, value := range snap.Hash {
				total += int64(len(field) + len(value))
			}
		case ValueSet:
			for _, member := range snap.Set {
				total += int64(len(member))
			}
		}
	}
	return total
}

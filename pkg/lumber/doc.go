// Package lumber provides a log classification engine that embeds log text
// into vectors and classifies against a 42-label taxonomy.
//
// Quick start:
//
//	l, err := lumber.New(lumber.WithModelDir("models/"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer l.Close()
//
//	event, _ := l.Classify("ERROR: connection refused to db-primary:5432")
//	fmt.Println(event.Type, event.Category) // ERROR connection_failure
//
// The Lumber instance is safe for concurrent use. Create once, reuse across
// requests. See the README for full documentation.
package lumber

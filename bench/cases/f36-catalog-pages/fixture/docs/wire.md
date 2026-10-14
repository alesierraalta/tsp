# Wire conventions

Index.From includes its lower bound and returns a detached sorted ID slice.
Label trims surrounding whitespace and returns the longest valid UTF-8 prefix whose encoded
length is no greater than its byte budget. It adds no ellipsis and rejects invalid UTF-8 input.
CSV exports use id,title,group columns. JSON imports validate the entire document before
constructing a store and reject duplicate IDs. Search is case-insensitive for titles and exact
for groups and tags. Replacing a record replaces its tags as well; tag order is retained.

# Project schema reference

The canonical public documentation for project.json is [PROJECT_FORMAT.md](PROJECT_FORMAT.md).

The current schema version is 3. Coordinates use a top-left origin, X to the right and Y downward. Rectangles are half-open: [x, x + width) × [y, y + height). Unknown fields are ignored for forward compatibility, and schema versions 1 and 2 are migrated in memory.

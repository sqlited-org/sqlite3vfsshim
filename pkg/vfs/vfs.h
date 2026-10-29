#ifndef SQLITE3VFS_H
#define SQLITE3VFS_H

#include <stdint.h>
#ifdef SQLITE3VFS_LOADABLE_EXT
/* Built as a SQLite loadable extension: SQLite's API is reached through the
** sqlite3_api_routines table instead of by linking against SQLite. */
#include "sqlite3ext.h"
#else
#include "sqlite3-binding.h"
#endif

/* Stores the sqlite3_api_routines pointer a loadable extension receives in its
** entry point (a no-op unless built with SQLITE3VFS_LOADABLE_EXT). */
void vfsSetAPI(const void *pApi);

/* sqlite3_uri_parameter() wrapper (a macro in loadable extension builds). */
//const char *vfsURIParameter(const char *zName, const char *zKey);

int vfsRegister(const char *zName, uintptr_t handle, int maxPathname, int makeDefault);
int vfsUnregister(const char *zName, uintptr_t *pHandle);

#endif

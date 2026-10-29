#ifndef SQLITE3VFSSHIM_H
#define SQLITE3VFSSHIM_H

#include <stdint.h>
#ifdef SQLITE3VFSSHIM_LOADABLE_EXT
/* Built as a SQLite loadable extension: SQLite's API is reached through the
** sqlite3_api_routines table instead of by linking against SQLite. */
#include "sqlite3ext.h"
#else
#include "sqlite3-binding.h"
#endif

/* Stores the sqlite3_api_routines pointer a loadable extension receives in its
** entry point (a no-op unless built with SQLITE3VFSSHIM_LOADABLE_EXT). */
void shimSetAPI(const void *pApi);

/* sqlite3_uri_parameter() wrapper (a macro in loadable extension builds). */
//const char *shimURIParameter(const char *zName, const char *zKey);

/* Bits telling which VFS-level hooks the Go shim implements. For a method with
** its bit set, C calls the Go hook first and then the parent; with the bit
** clear it only calls the parent. Must be passed to shimRegister. */
#define SHIM_V_OPEN     0x01
#define SHIM_V_DELETE   0x02
#define SHIM_V_ACCESS   0x04
#define SHIM_V_FULLPATH 0x08
#define SHIM_V_RANDOM   0x10
#define SHIM_V_SLEEP    0x20
#define SHIM_V_TIME     0x40

/* Same for the hooks of one open file; the Go side reports them from
** goShimOpen. */
#define SHIM_F_CLOSE    0x001
#define SHIM_F_READ     0x002
#define SHIM_F_WRITE    0x004
#define SHIM_F_TRUNCATE 0x008
#define SHIM_F_SYNC     0x010
#define SHIM_F_SIZE     0x020
#define SHIM_F_LOCK     0x040
#define SHIM_F_UNLOCK   0x080
#define SHIM_F_CHECKRES 0x100
#define SHIM_F_CONTROL  0x200

/* Registration. */
int shimRegister(const char *zName, const char *zParent, uintptr_t handle, int makeDefault, unsigned mask);
int shimUnregister(const char *zName, uintptr_t *pHandle);

#endif

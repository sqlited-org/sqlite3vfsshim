/*
** C side of the Go VFS shim.
**
** A shim VFS is a VFS that sits on top of another ("parent") VFS. The parent
** is known only here: Go supplies its name at registration, shimRegister()
** looks it up and keeps the sqlite3_vfs pointer, and every call to the parent
** is made from this file. Go only provides optional hooks.
**
** For each method, SQLite's call is handled like this:
**
**   1. if the Go side implements a hook for it (a SHIM_V_* bit for the VFS,
**      a SHIM_F_* bit for the open file), call the hook;
**   2. if the hook returned an error, return it to SQLite; the parent is not
**      called (the hook "vetoed" the call);
**   3. call the parent's method right here and return its result.
**
** Methods without a hook never enter Go, and methods the shim does not
** mention at all (dynamic loading, shared memory, mmap, system calls, ...) go
** straight to the parent.
**
** Every sqlite3_file created by the shim is laid out as
**
**     [ shimFile | padding | parent's sqlite3_file (parent->szOsFile bytes) ]
**
** so the parent's file lives in the same allocation SQLite gives us.
**
** Call flow, taking a write as example (the same applies to Read, Sync, Lock,
** ... and to the VFS-level methods):
**
**   SQLite
**     |  xWrite
**     v
**   shimWrite()            <- "sqlite3_io_methods" entry; SQLite -> shim
**     |  only if the File implements WriteHook
**     v
**   goShimFileWrite()      <- exported from Go (cgo); returns 0 or a veto code
**     |  File.WriteAt()
**     v
**   parent VFS xWrite      <- REAL(f)->pMethods->xWrite, called from C
**                             (e.g. the "unix" VFS writing to disk)
**
** Naming convention:
**   shimWrite, shimRead, shimOpen, ...   called BY SQLite, enter the shim
**   goShim*                              hooks implemented in Go (vfsshimcgo.go)
*/
#include "shim.h"
#include <stdlib.h>
#include <string.h>

#ifdef SQLITE3VFSSHIM_LOADABLE_EXT
SQLITE_EXTENSION_INIT1 /* defines the global sqlite3_api pointer */
void shimSetAPI(const void *pApi) { SQLITE_EXTENSION_INIT2((const sqlite3_api_routines *)pApi); }
#else
void shimSetAPI(const void *pApi) { (void)pApi; }
#endif

//const char *shimURIParameter(const char *zName, const char *zKey) { return sqlite3_uri_parameter(zName, zKey); }

/*
** ROUND8(x): round x up to the next multiple of 8.
**
** sqlite3_file objects are variable-sized: SQLite allocates vfs->szOsFile
** bytes per open file and hands us the pointer. We need to fit two structures
** in that block, our own shimFile header and, right behind it, the parent
** VFS's sqlite3_file (whose size is only known at runtime, parent->szOsFile).
**
** The parent's file struct may contain pointers, int64s or doubles, so it must
** start at a properly aligned address. Rounding the size of shimFile up to a
** multiple of 8 guarantees that the offset where the parent file starts is
** 8-byte aligned (SQLite itself uses the same ROUND8 idiom for this purpose).
** The same value is used in two places and they must agree:
**   - shimRegister():  szOsFile = ROUND8(sizeof(shimFile)) + parent->szOsFile
**   - REAL():          the offset at which the parent file is found
**
** Example: if sizeof(shimFile) is 20, ROUND8 gives 24, so the parent's
** sqlite3_file begins 24 bytes after the start of the block.
*/
#define ROUND8(x) (((x) + 7) & ~7)

typedef struct shimVfs {
  sqlite3_vfs base;
  sqlite3_vfs *parent;
  uintptr_t handle; /* cgo.Handle of the Go Shim */
  unsigned mask;    /* SHIM_V_* bits: methods implemented in Go */
} shimVfs;

typedef struct shimFile {
  sqlite3_file base;
  uintptr_t handle; /* cgo.Handle of the Go File, 0 if the file is a pure parent file */
  unsigned mask;    /* SHIM_F_* bits: methods implemented in Go */
} shimFile;

/*
** REAL(f): given the shim's sqlite3_file (the pointer SQLite gave us), return
** the embedded sqlite3_file belonging to the parent VFS.
**
** Memory layout of one open file (total size = vfs->szOsFile):
**
**   f --> +--------------------+  offset 0
**         | shimFile           |  our header: base.pMethods = shimIoMethods,
**         |                    |  plus the cgo handle of the Go File
**         +--------------------+  offset sizeof(shimFile)
**         | padding            |
**         +--------------------+  offset ROUND8(sizeof(shimFile))
**         | parent sqlite3_file|  REAL(f): initialised by parent->xOpen and
**         | (parent->szOsFile) |  driven through its own pMethods
**         +--------------------+
**
** Every shim method uses REAL(f) to call the parent file, shimOpen passes it
** to the parent's xOpen as the slot the parent file is opened into, and
** shimClose closes it exactly once. Because it lives inside SQLite's allocation, the
** parent file needs no separate malloc/free and is freed together with ours.
**
** Note: the cast to char* makes the addition byte-based rather than scaled by
** sizeof(shimFile).
*/
#define REAL(f) ((sqlite3_file *)((char *)(f) + ROUND8(sizeof(shimFile))))
#define SV(v) ((shimVfs *)(v))
#define PARENT(v) (SV(v)->parent)

/* Hooks implemented in Go (vfsshimcgo.go). A non-zero result vetoes the call. */
extern int goShimOpen(uintptr_t h, const char *zName, int flags, uintptr_t *pFile, unsigned *pMask);
extern int goShimDelete(uintptr_t h, const char *zName, int syncDir);
extern int goShimAccess(uintptr_t h, const char *zName, int flags);
extern int goShimFullPathname(uintptr_t h, const char *zName);
extern void goShimRandomness(uintptr_t h, int n, char *zOut);
extern void goShimSleep(uintptr_t h, int us);
extern int goShimCurrentTime(uintptr_t h);

extern int goShimFileClose(uintptr_t fh, int callClose);
extern int goShimFileRead(uintptr_t fh, void *buf, int amt, sqlite3_int64 ofst);
extern int goShimFileWrite(uintptr_t fh, const void *buf, int amt, sqlite3_int64 ofst);
extern int goShimFileTruncate(uintptr_t fh, sqlite3_int64 size);
extern int goShimFileSync(uintptr_t fh, int flags);
extern int goShimFileSize(uintptr_t fh);
extern int goShimFileLock(uintptr_t fh, int lock);
extern int goShimFileUnlock(uintptr_t fh, int lock);
extern int goShimFileCheckReservedLock(uintptr_t fh);
extern int goShimFileControl(uintptr_t fh, int op, void *arg);

/* ---------------------------------------------------------------- *
 * sqlite3_io_methods
 *
 * Direction: SQLite -> shim. SQLite calls these through f->pMethods. Each
 * one runs the Go hook of this open file if there is one (looked up through
 * the cgo handle stored in shimFile.handle) and then calls the parent file,
 * REAL(f), explicitly.
 * ---------------------------------------------------------------- */

/* HOOK(f, bit, call): run the Go hook `call` when the file implements it and
** return its error (veto) without touching the parent. */
#define GO(f, bit) (((shimFile *)(f))->mask & (bit))
#define HND(f) (((shimFile *)(f))->handle)
#define HOOK(f, bit, call) \
  do { if (GO(f, bit)) { int hrc_ = (call); if (hrc_ != SQLITE_OK) return hrc_; } } while (0)
#define PM(f) (REAL(f)->pMethods)

static int shimClose(sqlite3_file *f) {
  shimFile *sf = (shimFile *)f;
  sqlite3_file *real = REAL(f);
  int rc = SQLITE_OK;
  /* The Go side owns a handle whenever it returned a File; release it and run
  ** the Close hook only if the File implements it. */
  if (sf->handle) rc = goShimFileClose(sf->handle, (sf->mask & SHIM_F_CLOSE) != 0);
  /* The parent file is always closed, even if the hook failed. */
  if (real->pMethods) {
    int rc2 = real->pMethods->xClose(real);
    real->pMethods = 0;
    if (rc == SQLITE_OK) rc = rc2;
  }
  f->pMethods = 0;
  return rc;
}
static int shimRead(sqlite3_file *f, void *b, int n, sqlite3_int64 o) {
  HOOK(f, SHIM_F_READ, goShimFileRead(HND(f), b, n, o));
  return PM(f)->xRead(REAL(f), b, n, o);
}
static int shimWrite(sqlite3_file *f, const void *b, int n, sqlite3_int64 o) {
  HOOK(f, SHIM_F_WRITE, goShimFileWrite(HND(f), b, n, o));
  return PM(f)->xWrite(REAL(f), b, n, o);
}
static int shimTruncate(sqlite3_file *f, sqlite3_int64 s) {
  HOOK(f, SHIM_F_TRUNCATE, goShimFileTruncate(HND(f), s));
  return PM(f)->xTruncate(REAL(f), s);
}
static int shimSync(sqlite3_file *f, int fl) {
  HOOK(f, SHIM_F_SYNC, goShimFileSync(HND(f), fl));
  return PM(f)->xSync(REAL(f), fl);
}
static int shimFileSizeM(sqlite3_file *f, sqlite3_int64 *p) {
  HOOK(f, SHIM_F_SIZE, goShimFileSize(HND(f)));
  return PM(f)->xFileSize(REAL(f), p);
}
static int shimLock(sqlite3_file *f, int l) {
  HOOK(f, SHIM_F_LOCK, goShimFileLock(HND(f), l));
  return PM(f)->xLock(REAL(f), l);
}
static int shimUnlock(sqlite3_file *f, int l) {
  HOOK(f, SHIM_F_UNLOCK, goShimFileUnlock(HND(f), l));
  return PM(f)->xUnlock(REAL(f), l);
}
static int shimCheckReservedLock(sqlite3_file *f, int *p) {
  HOOK(f, SHIM_F_CHECKRES, goShimFileCheckReservedLock(HND(f)));
  return PM(f)->xCheckReservedLock(REAL(f), p);
}
static int shimFileControlM(sqlite3_file *f, int op, void *a) {
  HOOK(f, SHIM_F_CONTROL, goShimFileControl(HND(f), op, a));
  return PM(f)->xFileControl(REAL(f), op, a);
}
static int shimSectorSize(sqlite3_file *f) {
  sqlite3_file *r = REAL(f);
  return r->pMethods->xSectorSize ? r->pMethods->xSectorSize(r) : 4096;
}
static int shimDeviceCharacteristics(sqlite3_file *f) {
  sqlite3_file *r = REAL(f);
  return r->pMethods->xDeviceCharacteristics ? r->pMethods->xDeviceCharacteristics(r) : 0;
}

/* Returns the parent's open sqlite3_file. Used by the forwarders below. */
static sqlite3_file *parentFileOf(sqlite3_file *f) {
  sqlite3_file *r;
  r = REAL(f);
  return r->pMethods ? r : 0;
}

/* Shared memory (WAL), mmap: not intercepted, forwarded to the parent file.
** Without a parent file they fail (shm) or report "no mmap" (fetch). */
static int shimShmMap(sqlite3_file *f, int iPg, int pgsz, int bExtend, void volatile **pp) {
  sqlite3_file *r = parentFileOf(f);
  if (!r || r->pMethods->iVersion < 2 || !r->pMethods->xShmMap) return SQLITE_IOERR;
  return r->pMethods->xShmMap(r, iPg, pgsz, bExtend, pp);
}
static int shimShmLock(sqlite3_file *f, int off, int n, int flags) {
  sqlite3_file *r = parentFileOf(f);
  if (!r || r->pMethods->iVersion < 2 || !r->pMethods->xShmLock) return SQLITE_IOERR;
  return r->pMethods->xShmLock(r, off, n, flags);
}
static void shimShmBarrier(sqlite3_file *f) {
  sqlite3_file *r = parentFileOf(f);
  if (r && r->pMethods->iVersion >= 2 && r->pMethods->xShmBarrier) r->pMethods->xShmBarrier(r);
}
static int shimShmUnmap(sqlite3_file *f, int del) {
  sqlite3_file *r = parentFileOf(f);
  if (!r || r->pMethods->iVersion < 2 || !r->pMethods->xShmUnmap) return SQLITE_OK;
  return r->pMethods->xShmUnmap(r, del);
}
static int shimFetch(sqlite3_file *f, sqlite3_int64 off, int amt, void **pp) {
  sqlite3_file *r = parentFileOf(f);
  if (!r || r->pMethods->iVersion < 3 || !r->pMethods->xFetch) { *pp = 0; return SQLITE_OK; }
  return r->pMethods->xFetch(r, off, amt, pp);
}
static int shimUnfetch(sqlite3_file *f, sqlite3_int64 off, void *p) {
  sqlite3_file *r = parentFileOf(f);
  if (!r || r->pMethods->iVersion < 3 || !r->pMethods->xUnfetch) return SQLITE_OK;
  return r->pMethods->xUnfetch(r, off, p);
}

static const sqlite3_io_methods shimIoMethods = {
  3,
  shimClose, shimRead, shimWrite, shimTruncate, shimSync, shimFileSizeM,
  shimLock, shimUnlock, shimCheckReservedLock, shimFileControlM,
  shimSectorSize, shimDeviceCharacteristics,
  shimShmMap, shimShmLock, shimShmBarrier, shimShmUnmap,
  shimFetch, shimUnfetch
};

/* Used when the parent file implements only io-methods version 1 (no shared
** memory, no mmap): SQLite then cleanly refuses WAL mode instead of calling
** into missing methods. */
static const sqlite3_io_methods shimIoMethodsV1 = {
  1,
  shimClose, shimRead, shimWrite, shimTruncate, shimSync, shimFileSizeM,
  shimLock, shimUnlock, shimCheckReservedLock, shimFileControlM,
  shimSectorSize, shimDeviceCharacteristics
};

/* ---------------------------------------------------------------- *
 * sqlite3_vfs
 * ---------------------------------------------------------------- */

static int shimOpen(sqlite3_vfs *v, const char *zName, sqlite3_file *f, int flags, int *pOutFlags) {
  shimFile *sf = (shimFile *)f;
  sqlite3_file *real = REAL(f);
  int rc;
  int dummy;
  if (!pOutFlags) pOutFlags = &dummy;
  memset(sf, 0, sizeof(*sf));
  real->pMethods = 0;
  f->pMethods = 0;
  if (SV(v)->mask & SHIM_V_OPEN) {
    /* Hook first: it may veto, and returns the Go File with per-file hooks. */
    rc = goShimOpen(SV(v)->handle, zName, flags, &sf->handle, &sf->mask);
    if (rc != SQLITE_OK) return rc;
  }
  /* Then the parent opens the file, into the space behind our shimFile. */
  rc = PARENT(v)->xOpen(PARENT(v), zName, real, flags, pOutFlags);
  if (rc != SQLITE_OK) {
    real->pMethods = 0;
    if (sf->handle) goShimFileClose(sf->handle, 0); /* release the Go File */
    sf->handle = 0;
    return rc;
  }
  /* Advertise shared memory / mmap only if the parent file has them. */
  f->pMethods = real->pMethods->iVersion >= 2 ? &shimIoMethods : &shimIoMethodsV1;
  return SQLITE_OK;
}
/* VFS-level methods: Go hook first when the shim implements one, then the
** parent, called from C. */
static int shimDelete(sqlite3_vfs *v, const char *z, int sync) {
  if (SV(v)->mask & SHIM_V_DELETE) {
    int rc = goShimDelete(SV(v)->handle, z, sync);
    if (rc != SQLITE_OK) return rc;
  }
  return PARENT(v)->xDelete(PARENT(v), z, sync);
}
static int shimAccess(sqlite3_vfs *v, const char *z, int fl, int *p) {
  if (SV(v)->mask & SHIM_V_ACCESS) {
    int rc = goShimAccess(SV(v)->handle, z, fl);
    if (rc != SQLITE_OK) { *p = 0; return rc; }
  }
  return PARENT(v)->xAccess(PARENT(v), z, fl, p);
}
static int shimFullPathname(sqlite3_vfs *v, const char *z, int n, char *o) {
  if (SV(v)->mask & SHIM_V_FULLPATH) {
    int rc = goShimFullPathname(SV(v)->handle, z);
    if (rc != SQLITE_OK) return rc;
  }
  return PARENT(v)->xFullPathname(PARENT(v), z, n, o);
}
static int shimRandomness(sqlite3_vfs *v, int n, char *o) {
  if (SV(v)->mask & SHIM_V_RANDOM) goShimRandomness(SV(v)->handle, n, o);
  return PARENT(v)->xRandomness(PARENT(v), n, o);
}
static int shimSleep(sqlite3_vfs *v, int us) {
  if (SV(v)->mask & SHIM_V_SLEEP) goShimSleep(SV(v)->handle, us);
  return PARENT(v)->xSleep(PARENT(v), us);
}
/* The parent's current time in julian-day milliseconds, whichever of its
** two time methods it provides. */
static int parentCurrentTime(sqlite3_vfs *p, sqlite3_int64 *now) {
  if (p->iVersion >= 2 && p->xCurrentTimeInt64) return p->xCurrentTimeInt64(p, now);
  {
    double d = 0;
    int rc = p->xCurrentTime(p, &d);
    *now = (sqlite3_int64)(d * 86400000.0);
    return rc;
  }
}
static int shimCurrentTimeInt64(sqlite3_vfs *v, sqlite3_int64 *p) {
  if (SV(v)->mask & SHIM_V_TIME) {
    int rc = goShimCurrentTime(SV(v)->handle);
    if (rc != SQLITE_OK) return rc;
  }
  return parentCurrentTime(PARENT(v), p);
}
static int shimCurrentTime(sqlite3_vfs *v, double *p) {
  sqlite3_int64 t = 0;
  int rc = shimCurrentTimeInt64(v, &t);
  *p = t / 86400000.0;
  return rc;
}

/* Pure passthrough methods. */
/* Forwarded to the parent VFS (reporting "unsupported" if it lacks them). */
static void *shimDlOpen(sqlite3_vfs *v, const char *z) {
  sqlite3_vfs *par = PARENT(v);
  return par && par->xDlOpen ? par->xDlOpen(par, z) : 0;
}
static void shimDlError(sqlite3_vfs *v, int n, char *z) {
  sqlite3_vfs *par = PARENT(v);
  if (par && par->xDlError) par->xDlError(par, n, z);
  else sqlite3_snprintf(n, z, "dynamic loading is not supported by this VFS");
}
static void (*shimDlSym(sqlite3_vfs *v, void *h, const char *z))(void) {
  sqlite3_vfs *par = PARENT(v);
  return par && par->xDlSym ? par->xDlSym(par, h, z) : 0;
}
static void shimDlClose(sqlite3_vfs *v, void *h) {
  sqlite3_vfs *par = PARENT(v);
  if (par && par->xDlClose) par->xDlClose(par, h);
}
static int shimGetLastError(sqlite3_vfs *v, int n, char *z) {
  sqlite3_vfs *par = PARENT(v);
  return par && par->xGetLastError ? par->xGetLastError(par, n, z) : 0;
}
static int shimSetSystemCall(sqlite3_vfs *v, const char *z, sqlite3_syscall_ptr p) {
  sqlite3_vfs *par = PARENT(v);
  if (!par || par->iVersion < 3 || !par->xSetSystemCall) return SQLITE_NOTFOUND;
  return par->xSetSystemCall(par, z, p);
}
static sqlite3_syscall_ptr shimGetSystemCall(sqlite3_vfs *v, const char *z) {
  sqlite3_vfs *par = PARENT(v);
  if (!par || par->iVersion < 3 || !par->xGetSystemCall) return 0;
  return par->xGetSystemCall(par, z);
}
static const char *shimNextSystemCall(sqlite3_vfs *v, const char *z) {
  sqlite3_vfs *par = PARENT(v);
  if (!par || par->iVersion < 3 || !par->xNextSystemCall) return 0;
  return par->xNextSystemCall(par, z);
}

/* ---------------------------------------------------------------- *
 * Registration
 * ---------------------------------------------------------------- */

int shimRegister(const char *zName, const char *zParent, uintptr_t handle, int makeDefault, unsigned mask) {
  sqlite3_vfs *parent;
  shimVfs *sv;
  char *name;
  int rc;

  if (sqlite3_vfs_find(zName)) return SQLITE_MISUSE; /* name taken */
  /* Look up the named parent VFS; a NULL or empty name selects SQLite's default VFS. */
  parent = sqlite3_vfs_find(zParent && zParent[0] ? zParent : 0);
  if (!parent) return SQLITE_NOTFOUND;

  sv = (shimVfs *)calloc(1, sizeof(*sv));
  name = strdup(zName);
  if (!sv || !name) { 
    free(sv);
    free(name);
    return SQLITE_NOMEM; 
  }

  sv->parent = parent;
  sv->handle = handle;
  sv->mask = mask;
  sv->base.iVersion = 3;
  sv->base.szOsFile = ROUND8(sizeof(shimFile)) + parent->szOsFile;
  sv->base.mxPathname = parent->mxPathname;
  sv->base.zName = name;
  sv->base.pAppData = 0;
  sv->base.xOpen = shimOpen;
  sv->base.xDelete = shimDelete;
  sv->base.xAccess = shimAccess;
  sv->base.xFullPathname = shimFullPathname;
  sv->base.xDlOpen = shimDlOpen;
  sv->base.xDlError = shimDlError;
  sv->base.xDlSym = shimDlSym;
  sv->base.xDlClose = shimDlClose;
  sv->base.xRandomness = shimRandomness;
  sv->base.xSleep = shimSleep;
  sv->base.xCurrentTime = shimCurrentTime;
  sv->base.xGetLastError = shimGetLastError;
  sv->base.xCurrentTimeInt64 = shimCurrentTimeInt64;
  sv->base.xSetSystemCall = shimSetSystemCall;
  sv->base.xGetSystemCall = shimGetSystemCall;
  sv->base.xNextSystemCall = shimNextSystemCall;

  rc = sqlite3_vfs_register(&sv->base, makeDefault);
  if (rc != SQLITE_OK) { 
    free(name);
    free(sv);
  }
  return rc;
}

int shimUnregister(const char *zName, uintptr_t *pHandle) {
  sqlite3_vfs *v = sqlite3_vfs_find(zName);
  int rc;
  if (!v || v->xOpen != shimOpen) return SQLITE_NOTFOUND;
  rc = sqlite3_vfs_unregister(v);
  if (rc != SQLITE_OK) return rc;
  *pHandle = SV(v)->handle;
  free((void *)v->zName);
  free(v);
  return SQLITE_OK;
}

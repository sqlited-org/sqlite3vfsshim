/*
** C side of the Go VFS library: a complete sqlite3_vfs whose every method is
** implemented in Go (vfs.go / vfscgo.go).
**
** Call flow (SQLite -> Go only; there is no parent VFS to call back into):
**
**   SQLite xWrite -> vfsWrite() -> goVFSFileWrite() -> File.WriteAt()
**   SQLite xOpen  -> vfsOpen()  -> goVFSOpen()      -> VFS.Open()
**
** Go objects never live in C memory: the VFS and each File are cgo.Handle
** values (plain integers) stored in the C structs and passed back to Go.
*/
#include "vfs.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

#ifdef SQLITE3VFS_LOADABLE_EXT
SQLITE_EXTENSION_INIT1 /* defines the global sqlite3_api pointer */
void vfsSetAPI(const void *pApi) { SQLITE_EXTENSION_INIT2((const sqlite3_api_routines *)pApi); }
#else
void vfsSetAPI(const void *pApi) { (void)pApi; }
#endif

typedef struct vfsVfs {
  sqlite3_vfs base;
  uintptr_t handle; /* cgo.Handle of the Go VFS */
} vfsVfs;

typedef struct vfsFile {
  sqlite3_file base;
  uintptr_t handle; /* cgo.Handle of the Go File */
} vfsFile;

#define VH(v) (((vfsVfs *)(v))->handle)
#define FH(f) (((vfsFile *)(f))->handle)

/* Implemented in Go (vfscgo.go). */
extern int goVFSOpen(uintptr_t h, const char *zName, int flags, int *pOutFlags, uintptr_t *pFile);
extern int goVFSDelete(uintptr_t h, const char *zName, int syncDir);
extern int goVFSAccess(uintptr_t h, const char *zName, int flags, int *pRes);
extern int goVFSFullPathname(uintptr_t h, const char *zName, int nOut, char *zOut);
extern int goVFSRandomness(uintptr_t h, int n, char *zOut);
extern int goVFSSleep(uintptr_t h, int us);
extern int goVFSCurrentTime(uintptr_t h, sqlite3_int64 *pNow);

extern int goVFSFileClose(uintptr_t fh);
extern int goVFSFileRead(uintptr_t fh, void *buf, int amt, sqlite3_int64 ofst);
extern int goVFSFileWrite(uintptr_t fh, const void *buf, int amt, sqlite3_int64 ofst);
extern int goVFSFileTruncate(uintptr_t fh, sqlite3_int64 size);
extern int goVFSFileSync(uintptr_t fh, int flags);
extern int goVFSFileSize(uintptr_t fh, sqlite3_int64 *pSize);
extern int goVFSFileLock(uintptr_t fh, int lock);
extern int goVFSFileUnlock(uintptr_t fh, int lock);
extern int goVFSFileCheckReservedLock(uintptr_t fh, int *pRes);
extern int goVFSFileControl(uintptr_t fh, int op, void *arg);
extern int goVFSFileSectorSize(uintptr_t fh);
extern int goVFSFileDeviceCharacteristics(uintptr_t fh);

/* ---- sqlite3_io_methods (SQLite -> Go) ---- */

static int vfsClose(sqlite3_file *f) {
  int rc = goVFSFileClose(FH(f));
  f->pMethods = 0;
  return rc;
}
static int vfsRead(sqlite3_file *f, void *b, int n, sqlite3_int64 o) { return goVFSFileRead(FH(f), b, n, o); }
static int vfsWrite(sqlite3_file *f, const void *b, int n, sqlite3_int64 o) { return goVFSFileWrite(FH(f), b, n, o); }
static int vfsTruncate(sqlite3_file *f, sqlite3_int64 s) { return goVFSFileTruncate(FH(f), s); }
static int vfsSync(sqlite3_file *f, int fl) { return goVFSFileSync(FH(f), fl); }
static int vfsFileSize(sqlite3_file *f, sqlite3_int64 *p) { return goVFSFileSize(FH(f), p); }
static int vfsLock(sqlite3_file *f, int l) { return goVFSFileLock(FH(f), l); }
static int vfsUnlock(sqlite3_file *f, int l) { return goVFSFileUnlock(FH(f), l); }
static int vfsCheckReservedLock(sqlite3_file *f, int *p) { return goVFSFileCheckReservedLock(FH(f), p); }
static int vfsFileControl(sqlite3_file *f, int op, void *a) { return goVFSFileControl(FH(f), op, a); }
static int vfsSectorSize(sqlite3_file *f) { return goVFSFileSectorSize(FH(f)); }
static int vfsDeviceCharacteristics(sqlite3_file *f) { return goVFSFileDeviceCharacteristics(FH(f)); }

/* Version 1: no shared memory (WAL) and no mmap; SQLite refuses WAL cleanly. */
static const sqlite3_io_methods vfsIoMethods = {
  1,
  vfsClose, vfsRead, vfsWrite, vfsTruncate, vfsSync, vfsFileSize,
  vfsLock, vfsUnlock, vfsCheckReservedLock, vfsFileControl,
  vfsSectorSize, vfsDeviceCharacteristics
};

/* ---- sqlite3_vfs (SQLite -> Go) ---- */

static int vfsOpen(sqlite3_vfs *v, const char *zName, sqlite3_file *f, int flags, int *pOutFlags) {
  int dummy;
  int rc;

  vfsFile *vf = (vfsFile *)f;
  memset(vf, 0, sizeof(*vf));
  if (!pOutFlags) pOutFlags = &dummy;
  rc = goVFSOpen(VH(v), zName, flags, pOutFlags, &vf->handle);
  if (rc != SQLITE_OK) { f->pMethods = 0; return rc; }
  f->pMethods = &vfsIoMethods;
  return SQLITE_OK;
}
static int vfsDelete(sqlite3_vfs *v, const char *z, int sync) { return goVFSDelete(VH(v), z, sync); }
static int vfsAccess(sqlite3_vfs *v, const char *z, int fl, int *p) { return goVFSAccess(VH(v), z, fl, p); }
static int vfsFullPathname(sqlite3_vfs *v, const char *z, int n, char *o) { return goVFSFullPathname(VH(v), z, n, o); }
static int vfsRandomness(sqlite3_vfs *v, int n, char *o) { return goVFSRandomness(VH(v), n, o); }
static int vfsSleep(sqlite3_vfs *v, int us) { return goVFSSleep(VH(v), us); }
static int vfsCurrentTimeInt64(sqlite3_vfs *v, sqlite3_int64 *p) { return goVFSCurrentTime(VH(v), p); }
static int vfsCurrentTime(sqlite3_vfs *v, double *p) {
  sqlite3_int64 t = 0;
  int rc = goVFSCurrentTime(VH(v), &t);
  *p = t / 86400000.0;
  return rc;
}

/* Dynamic loading is not supported by Go VFSes. */
static void *vfsDlOpen(sqlite3_vfs *v, const char *z) { return 0; }
static void vfsDlError(sqlite3_vfs *v, int n, char *z) { sqlite3_snprintf(n, z, "dynamic loading is not supported by this VFS"); }
static void (*vfsDlSym(sqlite3_vfs *v, void *h, const char *z))(void) { return 0; }
static void vfsDlClose(sqlite3_vfs *v, void *h) {}

int vfsRegister(const char *zName, uintptr_t handle, int maxPathname, int makeDefault) {
  vfsVfs *vv;
  char *name;
  int rc;

  if (sqlite3_vfs_find(zName)) return SQLITE_MISUSE; /* name taken */
  vv = (vfsVfs *)calloc(1, sizeof(*vv));
  name = strdup(zName);
  if (!vv || !name) { free(vv); free(name); return SQLITE_NOMEM; }

  vv->handle = handle;
  vv->base.iVersion = 2;
  vv->base.szOsFile = sizeof(vfsFile);
  vv->base.mxPathname = maxPathname;
  vv->base.zName = name;
  vv->base.xOpen = vfsOpen;
  vv->base.xDelete = vfsDelete;
  vv->base.xAccess = vfsAccess;
  vv->base.xFullPathname = vfsFullPathname;
  vv->base.xDlOpen = vfsDlOpen;
  vv->base.xDlError = vfsDlError;
  vv->base.xDlSym = vfsDlSym;
  vv->base.xDlClose = vfsDlClose;
  vv->base.xRandomness = vfsRandomness;
  vv->base.xSleep = vfsSleep;
  vv->base.xCurrentTime = vfsCurrentTime;
  vv->base.xGetLastError = 0;
  vv->base.xCurrentTimeInt64 = vfsCurrentTimeInt64;

  rc = sqlite3_vfs_register(&vv->base, makeDefault);
  if (rc != SQLITE_OK) { free(name); free(vv); }
  return rc;
}

int vfsUnregister(const char *zName, uintptr_t *pHandle) {
  sqlite3_vfs *v = sqlite3_vfs_find(zName);
  int rc;
  if (!v || v->xOpen != vfsOpen) return SQLITE_NOTFOUND;
  rc = sqlite3_vfs_unregister(v);
  if (rc != SQLITE_OK) return rc;
  *pHandle = VH(v);
  free((void *)v->zName);
  free(v);
  return SQLITE_OK;
}

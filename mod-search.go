package djs

// #include "duktape.h"
// extern duk_ret_t modSearch(duk_context *ctx);
// static const char *getCString(duk_context *ctx, duk_idx_t idx);
import "C"
import (
	"strings"
	"fmt"
	"os"
	"path"
)

var (
	mod_path = "\xFFmodPath"
)

//export modSearch
func modSearch(ctx *C.duk_context) C.duk_ret_t {
	/* Nargs was given as 4 and we get the following stack arguments:
	 *   index 0: id
	 *   index 1: require
	 *   index 2: exports
	 *   index 3: module
	 */
	modPath := C.GoString(C.getCString(ctx, 0))
	if !strings.HasSuffix(modPath, ".js") {
		modPath = fmt.Sprintf("%s.js", modPath)
	}

	modHome := getModuleHome(ctx)
	absModPath := toAbsPath(modHome, modPath)
	b, err := os.ReadFile(absModPath)
	if err != nil {
		return 0
	}

	var src *C.char
	var size C.int
	getBytesPtrLen(b, &src, &size)

	C.duk_push_lstring(ctx, src, C.size_t(size))
	return 1
}

func setObjFunction(ctx *C.duk_context, funcName string, fn C.duk_c_function, nargs int) {
	var cFuncName *C.char
	var funcNameLen C.int
	getStrPtrLen(&funcName, &cFuncName, &funcNameLen)

	// [ obj ]
	C.duk_push_lstring(ctx, cFuncName, C.size_t(funcNameLen))  // [ obj funcName ]
	C.duk_push_c_function(ctx, fn, C.duk_idx_t(nargs)) // [ obj funcName fn ]
	C.duk_put_prop(ctx, -3)  // [ obj ] with obj[funcName]=fn
}

func setModSearch(ctx *C.duk_context, moduleHome string) {
	setModuleHome(ctx, moduleHome)

	duktape := "Duktape"
	var cDuktape *C.char
	var length C.int
	getStrPtrLen(&duktape, &cDuktape, &length)

	C.duk_get_global_lstring(ctx, cDuktape, C.size_t(length))
	setObjFunction(ctx, "modSearch", (C.duk_c_function)(C.modSearch), 4)
	C.duk_pop(ctx)
}

func setModuleHome(ctx *C.duk_context, moduleHome string) {
	C.duk_push_global_object(ctx); // [ global ]
	defer C.duk_pop(ctx) // [ ]

	var name *C.char
	var length C.int
	getStrPtrLen(&mod_path, &name, &length)
	C.duk_push_lstring(ctx, name, C.size_t(length)) // [ global mod_path ]

	var home string
	if len(moduleHome) > 0 {
		home = moduleHome
	} else {
		home = exePath
	}
	getStrPtrLen(&home, &name, &length)
	C.duk_push_lstring(ctx, name, C.size_t(length)) // [ global mod_path home ]
	C.duk_put_prop(ctx, -3)  // [ global ] with global[mod_path] = home
}

func getModuleHome(ctx *C.duk_context) string {
	var modHome string

	var name *C.char
	var length C.int
	getStrPtrLen(&mod_path, &name, &length)
	C.duk_get_global_lstring(ctx, name, C.size_t(length)) // [ home ]
	if val, err := fromJsValue(ctx); err == nil {
		modHome = fmt.Sprintf("%s", val.(string))
	} else {
		modHome = exePath
	}
	C.duk_pop(ctx) // [ ]
	return modHome
}

func getExecWD() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return path.Dir(exePath), nil
}

func toAbsPath(absRoot, filePath string) string {
	if path.IsAbs(filePath) {
		return filePath
	}
	return path.Join(absRoot, filePath)
}

var exePath string
func init() {
	p, e := getExecWD()
	if e != nil {
		panic(e)
	}
	exePath = p
}

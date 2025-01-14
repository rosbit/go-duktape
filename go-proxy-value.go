package djs

// #include "duktape.h"
// extern duk_ret_t go_obj_get(duk_context *ctx);
// extern duk_ret_t go_obj_set(duk_context *ctx);
// extern duk_ret_t go_obj_has(duk_context *ctx);
// extern duk_ret_t go_func_apply(duk_context *ctx);
// extern duk_ret_t goDummyFunc(duk_context *ctx);
// extern duk_ret_t freeTarget(duk_context *ctx);
import "C"
import (
	elutils "github.com/rosbit/go-embedding-utils"
	"encoding/json"
	"reflect"
	"unsafe"
	"math"
	"fmt"
	"strings"
)

func pushJsProxyValue(ctx *C.duk_context, v interface{}) {
	if v == nil {
		C.duk_push_null(ctx)
		return
	}

	vv := reflect.ValueOf(v)
	switch vv.Kind() {
	case reflect.Bool:
		if v.(bool) {
			C.duk_push_true(ctx)
		} else {
			C.duk_push_false(ctx)
		}
		return
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32/*, reflect.Int64*/:
		C.duk_push_number(ctx, C.duk_double_t(vv.Int()))
		return
	case reflect.Uint,reflect.Uint8,reflect.Uint16,reflect.Uint32/*,reflect.Uint64*/:
		C.duk_push_number(ctx, C.duk_double_t(vv.Uint()))
		return
	case reflect.Int64, reflect.Uint64:
		pushString(ctx, fmt.Sprintf("%v", v))
		return
	case reflect.Float32, reflect.Float64:
		fv := vv.Float()
		if math.IsNaN(fv) {
			C.duk_push_nan(ctx)
			return
		}
		C.duk_push_number(ctx, C.duk_double_t(fv))
		return
	case reflect.String:
		if (vv.Type().String() == "json.Number") {
			n := v.(json.Number)
			pushString(ctx, n.String())
			/*
			if n64, e := n.Int64(); e == nil {
				C.duk_push_number(ctx, C.duk_double_t(n64))
				return
			}
			f64, _ := n.Float64()
			C.duk_push_number(ctx, C.duk_double_t(f64))
			*/
			return
		}
		pushString(ctx, v.(string))
		return
	case reflect.Slice:
		t := vv.Type()
		if t.Elem().Kind() == reflect.Uint8 {
			pushString(ctx, string(v.([]byte)))
			return
		}
		fallthrough
	case reflect.Array:
		pushGoArray(ctx, v)
		return
	case reflect.Map, reflect.Struct, reflect.Interface:
		pushGoObj(ctx, v)
		return
	case reflect.Ptr:
		if vv.Elem().Kind() == reflect.Struct {
			pushGoObj(ctx, v)
			return
		}
		pushJsProxyValue(ctx, vv.Elem().Interface())
		return
	case reflect.Func:
		pushGoFunc(ctx, v)
		return
	default:
		C.duk_push_undefined(ctx)
		return
	}
}

func getTargetIdx(ctx *C.duk_context, targetIdx ...C.duk_idx_t) (idx uint32, isProxy bool) {
	// [ 0 ] target if no targetIdx
	// ...
	var tIdx C.duk_idx_t
	if len(targetIdx) > 0 {
		tIdx = targetIdx[0]
	}

	var name *C.char
	getStrPtr(&idxName, &name)
	isProxy = C.duk_get_prop_string(ctx, tIdx, name) != 0 // [ ... idx/undefined ]
	if isProxy {
		idx = uint32(C.duk_get_uint(ctx, -1))
	}
	C.duk_pop(ctx) // [ ... ]
	return
}

func getTargetValue(ctx *C.duk_context, targetIdx ...C.duk_idx_t) (v interface{}, isProxy bool) {
	// [ 0 ] target if no targetIdx
	// ....
	var idx uint32
	if idx, isProxy = getTargetIdx(ctx, targetIdx...); !isProxy {
		return
	}

	ptr := getPtrStore(uintptr(unsafe.Pointer(ctx)))
	vPtr, o := ptr.lookup(idx)
	if !o {
		isProxy = false
		return
	}
	if vv, o := vPtr.(*interface{}); o {
		v = *vv
	}
	return
}

func go_arr_get(ctx *C.duk_context, vv reflect.Value) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: receiver (proxy)
	 */
	if C.duk_is_string(ctx, 1) != 0 {
		key := C.GoString(C.duk_get_string(ctx, 1))
		if key == "length" {
			C.duk_push_int(ctx, C.duk_int_t(vv.Len()))
			return 1
		}
		C.duk_push_undefined(ctx)
		return 1
	}
	if C.duk_is_number(ctx, 1) == 0 {
		C.duk_push_undefined(ctx)
		return 1
	}
	key := int(C.duk_to_int(ctx, 1))
	l := vv.Len()
	if key < 0 || key >= l {
		C.duk_push_undefined(ctx)
		return 1
	}
	val := vv.Index(key)
	if !val.IsValid() || !val.CanInterface() {
		C.duk_push_undefined(ctx)
		return 1
	}
	pushJsProxyValue(ctx, val.Interface())
	return 1
}

func go_arr_set(ctx *C.duk_context, vv reflect.Value) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: val
	 * [3]: receiver (proxy)
	 */
	if C.duk_is_number(ctx, 1) == 0 {
		C.duk_push_false(ctx)
		return 1
	}
	key := int(C.duk_to_int(ctx, 1))

	C.duk_dup(ctx, 2) // [ ... val ]
	goVal, err := fromJsValue(ctx)
	C.duk_pop(ctx)    // [ ... ]
	if err != nil {
		C.duk_push_false(ctx)
		return 1
	}

	l := vv.Len()
	if key < 0 || key >= l {
		C.duk_push_false(ctx)
		return 1
	}
	dest := vv.Index(key)
	if _, ok := goVal.(string); ok {
		goVal = fmt.Sprintf("%s", goVal) // deep copy
	}
	if err = elutils.SetValue(dest, goVal); err != nil {
		C.duk_push_false(ctx)
	} else {
		C.duk_push_true(ctx)
	}
	return 1
}

func go_arr_has(ctx *C.duk_context, vv reflect.Value) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 */
	if C.duk_is_string(ctx, 1) != 0 {
		key := C.GoString(C.duk_get_string(ctx, 1))
		if key == "length" {
			C.duk_push_true(ctx)
			return 1
		}
		C.duk_push_false(ctx)
		return 1
	}
	if C.duk_is_number(ctx, 1) == 0 {
		C.duk_push_false(ctx)
		return 1
	}
	key := int(C.duk_to_int(ctx, 1))
	l := vv.Len()
	if key < 0 || key >= l {
		C.duk_push_false(ctx)
		return 1
	}
	C.duk_push_true(ctx)
	return 1
}

func go_map_get(ctx *C.duk_context, vv reflect.Value) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: receiver (proxy)
	 */
	if C.duk_is_string(ctx, 1) == 0 {
		C.duk_push_undefined(ctx)
		return 1
	}
	key := C.GoString(C.duk_get_string(ctx, 1))
	val := vv.MapIndex(reflect.ValueOf(key))
	if !val.IsValid() || !val.CanInterface() {
		C.duk_push_undefined(ctx)
		return 1
	}
	pushJsProxyValue(ctx, val.Interface())
	return 1
}

func go_map_set(ctx *C.duk_context, vv reflect.Value) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: val
	 * [3]: receiver (proxy)
	 */
	if C.duk_is_string(ctx, 1) == 0 {
		C.duk_push_false(ctx)
		return 1
	}
	key := C.GoString(C.duk_get_string(ctx, 1))

	C.duk_dup(ctx, 2) // [ ... val ]
	goVal, err := fromJsValue(ctx)
	C.duk_pop(ctx)    // [ ... ]
	if err != nil {
		C.duk_push_false(ctx)
		return 1
	}

	mapT := vv.Type()
	elType := mapT.Elem()
	dest := elutils.MakeValue(elType)
	if _, ok := goVal.(string); ok {
		goVal = fmt.Sprintf("%s", goVal) // deep copy
	}
	if err = elutils.SetValue(dest, goVal); err == nil {
		vv.SetMapIndex(reflect.ValueOf(key), dest)
		C.duk_push_true(ctx)
	} else {
		C.duk_push_false(ctx)
	}
	return 1
}

func go_map_has(ctx *C.duk_context, vv reflect.Value) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 */
	if C.duk_is_string(ctx, 1) == 0 {
		C.duk_push_false(ctx)
		return 1
	}
	key := C.GoString(C.duk_get_string(ctx, 1))
	val := vv.MapIndex(reflect.ValueOf(key))
	if !val.IsValid() {
		C.duk_push_false(ctx)
	} else {
		C.duk_push_true(ctx)
	}
	return 1
}

func go_struct_get(ctx *C.duk_context, structVar reflect.Value) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: receiver (proxy)
	 */
	if C.duk_is_string(ctx, 1) == 0 {
		C.duk_push_undefined(ctx)
		return 1
	}
	key := C.GoString(C.duk_get_string(ctx, 1))
	var structE reflect.Value
	switch structVar.Kind() {
	case reflect.Struct:
		structE = structVar
	case reflect.Ptr:
		if structVar.Elem().Kind() != reflect.Struct {
			C.duk_push_undefined(ctx)
			return 1
		}
		structE = structVar.Elem()
	default:
		C.duk_push_undefined(ctx)
		return 1
	}
	name := upperFirst(key)
	fv := structE.FieldByName(name)
	if !fv.IsValid() {
		fv = structE.MethodByName(name)
		if !fv.IsValid() {
			if structE == structVar {
				C.duk_push_undefined(ctx)
				return 1
			}
			fv = structVar.MethodByName(name)
			if !fv.IsValid() {
				C.duk_push_undefined(ctx)
				return 1
			}
		}
		if fv.CanInterface() {
			pushGoFunc(ctx, fv.Interface())
			return 1
		}
		C.duk_push_undefined(ctx)
		return 1
	}
	if !fv.CanInterface() {
		C.duk_push_undefined(ctx)
		return 1
	}
	pushJsProxyValue(ctx, fv.Interface())
	return 1
}

func go_struct_set(ctx *C.duk_context, vv reflect.Value) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: val
	 * [3]: receiver (proxy)
	 */
	if C.duk_is_string(ctx, 1) == 0 {
		C.duk_push_false(ctx)
		return 1
	}
	key := C.GoString(C.duk_get_string(ctx, 1))

	C.duk_dup(ctx, 2) // [ ... val ]
	goVal, err := fromJsValue(ctx)
	C.duk_pop(ctx)    // [ ... ]
	if err != nil {
		C.duk_push_false(ctx)
		return 1
	}

	var structE reflect.Value
	switch vv.Kind() {
	case reflect.Struct:
		structE = vv
	case reflect.Ptr:
		if vv.Elem().Kind() != reflect.Struct {
			C.duk_push_undefined(ctx)
			return 1
		}
		structE = vv.Elem()
	default:
		C.duk_push_false(ctx)
		return 1
	}
	name := upperFirst(key)
	fv := structE.FieldByName(name)
	if !fv.IsValid() {
		C.duk_push_false(ctx)
		return 1
	}
	if _, ok := goVal.(string); ok {
		goVal = fmt.Sprintf("%s", goVal) // deep copy
	}
	if err = elutils.SetValue(fv, goVal); err != nil {
		C.duk_push_false(ctx)
		return 1
	}
	C.duk_push_true(ctx)
	return 1
}

func go_struct_has(ctx *C.duk_context, vv reflect.Value) C.duk_ret_t {
	// 'this' binding: handler
	// [0]: target
	// [1]: key
	if C.duk_is_string(ctx, 1) == 0 {
		C.duk_push_false(ctx)
		return 1
	}
	key := C.GoString(C.duk_get_string(ctx, 1))

	var structE reflect.Value
	switch vv.Kind() {
	case reflect.Struct:
		structE = vv
	case reflect.Ptr:
		if vv.Elem().Kind() != reflect.Struct {
			C.duk_push_false(ctx)
			return 1
		}
		structE = vv.Elem()
	default:
		C.duk_push_false(ctx)
		return 1
	}
	name := upperFirst(key)
	fv := structE.FieldByName(name)
	if !fv.IsValid() {
		C.duk_push_false(ctx)
		return 1
	}
	C.duk_push_true(ctx)
	return 1
}

func go_interface_get(ctx *C.duk_context, vv reflect.Value) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: receiver (proxy)
	 */
	if C.duk_is_string(ctx, 1) == 0 {
		C.duk_push_undefined(ctx)
		return 1
	}
	key := C.GoString(C.duk_get_string(ctx, 1))
	name := upperFirst(key)
	fv := vv.MethodByName(name)
	if !fv.IsValid() || !fv.CanInterface() {
		C.duk_push_undefined(ctx)
		return 1
	}
	pushGoFunc(ctx, fv.Interface())
	return 1
}

//export go_obj_get
func go_obj_get(ctx *C.duk_context) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: receiver (proxy)
	 */
	v, isProxy := getTargetValue(ctx)
	if !isProxy {
		C.duk_push_undefined(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_undefined(ctx)
		return 1
	}
	switch vv := reflect.ValueOf(v); vv.Kind() {
	case reflect.Slice, reflect.Array:
		return go_arr_get(ctx, vv)
	case reflect.Map:
		return go_map_get(ctx, vv)
	case reflect.Struct, reflect.Ptr:
		return go_struct_get(ctx, vv)
	case reflect.Interface:
		return go_interface_get(ctx, vv)
	default:
		C.duk_push_undefined(ctx)
		return 1
	}
}

//export go_obj_set
func go_obj_set(ctx *C.duk_context) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: val
	 * [3]: receiver (proxy)
	 */
	v, isProxy := getTargetValue(ctx)
	if !isProxy {
		C.duk_push_false(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_false(ctx)
		return 1
	}
	switch vv := reflect.ValueOf(v); vv.Kind() {
	case reflect.Slice, reflect.Array:
		return go_arr_set(ctx, vv)
	case reflect.Map:
		return go_map_set(ctx, vv)
	case reflect.Struct, reflect.Ptr:
		return go_struct_set(ctx, vv)
	default:
		C.duk_push_false(ctx)
		return 1
	}
}

//export go_obj_has
func go_obj_has(ctx *C.duk_context) C.duk_ret_t {
	// 'this' binding: handler
	// [0]: target
	// [1]: key
	v, isProxy := getTargetValue(ctx)
	if !isProxy {
		C.duk_push_false(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_false(ctx)
		return 1
	}
	switch vv := reflect.ValueOf(v); vv.Kind() {
	case reflect.Slice, reflect.Array:
		return go_arr_has(ctx, vv)
	case reflect.Map:
		return go_map_has(ctx, vv)
	case reflect.Struct, reflect.Ptr:
		return go_struct_has(ctx, vv)
	default:
		C.duk_push_false(ctx)
		return 1
	}
}

//export goDummyFunc
func goDummyFunc(ctx *C.duk_context) C.duk_ret_t {
	return 0
}

//export go_func_apply
func go_func_apply(ctx *C.duk_context) C.duk_ret_t {
	// 'this' binding: handler
	// [0]: target
	// [1]: receiver
	// [2]: args-array
	fn, isProxy := getTargetValue(ctx)
	if !isProxy {
		return C.DUK_RET_ERROR
	}
	if fn == nil {
		return C.DUK_RET_ERROR
	}
	fnVal := reflect.ValueOf(fn)
	if fnVal.Kind() != reflect.Func {
		return C.DUK_RET_ERROR
	}
	fnType := fnVal.Type()

	// make args for Golang function
	argc := int(C.duk_get_length(ctx, 2))
	helper := elutils.NewGolangFuncHelperDirectly(fnVal, fnType)
	getArgs := func(i int) interface{} {
		C.duk_get_prop_index(ctx, 2, C.duk_uarridx_t(i)) // [ ... i-th arg ]
		defer C.duk_pop(ctx) // [ ... ]

		if goVal, err := fromJsValue(ctx); err == nil {
			return goVal
		}
		return nil
	}
	v, e := helper.CallGolangFunc(argc, "djs-func", getArgs) // call Golang function

	// convert result (in var v) of Golang function to that of JS.
	// 1. error
	if e != nil {
		return C.DUK_RET_ERROR
	}

	// 2. no result
	if v == nil {
		return 0 // undefined
	}

	// 3. array or scalar
	pushJsProxyValue(ctx, v) // [ args ... v ]
	return 1
}

//export freeTarget
func freeTarget(ctx *C.duk_context) C.duk_ret_t {
	// Object being finalized is at stack index 0
	if idx, isProxy := getTargetIdx(ctx); isProxy {
		// fmt.Printf("--- freeTarget is called\n")
		ptr := getPtrStore(uintptr(unsafe.Pointer(ctx)))
		ptr.remove(idx)
	}
	return 0
}

func makeProxyObject(ctx *C.duk_context, v interface{}, proxyHandlerName string) {
	var name *C.char

	// [ target ]
	ptr := getPtrStore(uintptr(unsafe.Pointer(ctx)))
	idx := ptr.register(&v)
	C.duk_push_uint(ctx, C.duk_uint_t(idx)) // [ target idx ]
	getStrPtr(&idxName, &name)
	C.duk_put_prop_string(ctx, -2, name)  // [ target ] with taget[name] = idx

	C.duk_push_c_function(ctx, (*[0]byte)(C.freeTarget), 1); // [ target finalizer ]
	C.duk_set_finalizer(ctx, -2); // [ target ] with finilizer = freeTarget

	getStrPtr(&proxyHandlerName, &name)
	C.duk_get_global_string(ctx, name) // [ target handler ]

	C.duk_push_proxy(ctx, 0) // [ Proxy(target,handler) ]
}

func pushGoArray(ctx *C.duk_context, v interface{}) {
	C.duk_push_bare_array(ctx)
	makeProxyObject(ctx, v, goObjProxyHandler)
}

func pushGoObj(ctx *C.duk_context, v interface{}) {
	C.duk_push_bare_object(ctx)
	makeProxyObject(ctx, v, goObjProxyHandler)
}

func pushGoFunc(ctx *C.duk_context, fnVar interface{}) {
	fnType := reflect.TypeOf(fnVar)
	argc := fnType.NumIn()
	nargs := C.int(C.DUK_VARARGS)
	if !fnType.IsVariadic() {
		nargs = C.int(argc)
	}
	C.duk_push_c_function(ctx, (C.duk_c_function)(C.goDummyFunc), nargs) // [ target ] goDummyFunc as target
	makeProxyObject(ctx, fnVar, goFuncProxyHandler)
}

type trapFunc struct {
	name string
	fn C.duk_c_function
	nargs C.duk_idx_t
}
func registerProxyHandler(ctx *C.duk_context, proxyHandlerName string, trapFuncs ...*trapFunc) {
	var name *C.char

	C.duk_push_bare_object(ctx)  // [ handler ]

	for _, trap := range trapFuncs {
		C.duk_push_c_function(ctx, trap.fn, trap.nargs) // [ handler trap-func ]
		getStrPtr(&trap.name, &name)
		C.duk_put_prop_string(ctx, -2, name) // [ handler ] with handler[trap-name] = trap-func
	}

	getStrPtr(&proxyHandlerName, &name)
	C.duk_put_global_string(ctx, name) // [ ] with global[proxyHandlerName] = handler
}

func registerGoProxyHandlers(ctx *C.duk_context) {
	registerProxyHandler(ctx, goObjProxyHandler, &trapFunc{
		name: get, fn: (C.duk_c_function)(C.go_obj_get), nargs: 3,
	}, &trapFunc{
		name: set, fn: (C.duk_c_function)(C.go_obj_set), nargs: 4,
	}, &trapFunc{
		name: has, fn: (C.duk_c_function)(C.go_obj_has), nargs: 2,
	})

	registerProxyHandler(ctx, goFuncProxyHandler, &trapFunc{
		name: apply, fn: (C.duk_c_function)(C.go_func_apply), nargs: 3,
	})
}

func pushString(ctx *C.duk_context, s string) {
	var cstr *C.char
	var sLen C.int
	getStrPtrLen(&s, &cstr, &sLen)
	C.duk_push_lstring(ctx, cstr, C.size_t(sLen))
}

func upperFirst(name string) string {
	return strings.ToUpper(name[:1]) + name[1:]
}


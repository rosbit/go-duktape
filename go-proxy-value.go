package djs

// #include "duktape.h"
// extern duk_ret_t go_arr_handle_get(duk_context *ctx);
// extern duk_ret_t go_arr_handle_set(duk_context *ctx);
// extern duk_ret_t go_map_handle_get(duk_context *ctx);
// extern duk_ret_t go_map_handle_set(duk_context *ctx);
// extern duk_ret_t go_struct_handle_get(duk_context *ctx);
// extern duk_ret_t go_struct_handle_set(duk_context *ctx);
// extern duk_ret_t go_struct_handle_has(duk_context *ctx);
// extern duk_ret_t go_struct_handle_ownKeys(duk_context *ctx);
// // extern duk_ret_t goProxyFuncBridge(duk_context *ctx);
// // extern duk_ret_t go_func_handler_apply(duk_context *ctx);
// // extern duk_ret_t go_var_handle_get(duk_context *ctx);
// // extern duk_ret_t go_var_handle_set(duk_context *ctx);
import "C"
import (
	elutils "github.com/rosbit/go-embedding-utils"
	"reflect"
	"unsafe"
	"fmt"
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
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		C.duk_push_number(ctx, C.duk_double_t(vv.Int()))
		return
	case reflect.Uint,reflect.Uint8,reflect.Uint16,reflect.Uint32,reflect.Uint64:
		C.duk_push_number(ctx, C.duk_double_t(vv.Uint()))
		return
	case reflect.Float32, reflect.Float64:
		C.duk_push_number(ctx, C.duk_double_t(vv.Float()))
		return
	case reflect.String:
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
		pushArrProxy(ctx, v)
		return
	case reflect.Map:
		pushMapProxy(ctx, v)
		return
	case reflect.Struct:
		pushStructProxy(ctx, v)
		return
	case reflect.Ptr:
		if vv.Elem().Kind() == reflect.Struct {
			pushStructProxy(ctx, v)
			return
		}
		pushJsProxyValue(ctx, vv.Elem().Interface())
		return
	case reflect.Func:
		// pushGoFuncProxy(ctx, v)
		pushGoFunc(ctx, v)
		return
	default:
		// return fmt.Errorf("unsupported type %v", vv.Kind())
		C.duk_push_undefined(ctx)
		return
	}
}

func getTargetValue(ctx *C.duk_context, targetIdx ...C.duk_idx_t) (v interface{}, ok bool) {
	// [ 0 ] target if no targetIdx
	// ....
	var tIdx C.duk_idx_t
	if len(targetIdx) > 0 {
		tIdx = targetIdx[0]
	}

	var name *C.char
	getStrPtr(&idxName, &name)
	C.duk_get_prop_string(ctx, tIdx, name) // [ ... idx ]
	idx := int(C.duk_to_int(ctx, -1))
	C.duk_pop(ctx) // [ ... ]

	ptr := getPtrStore(uintptr(unsafe.Pointer(ctx)))
	vPtr, o := ptr.lookup(idx)
	if !o {
		return
	}
	if vv, o := vPtr.(*interface{}); o {
		v = *vv
		ok = true
	}
	return
}

//export go_arr_handle_get
func go_arr_handle_get(ctx *C.duk_context) C.duk_ret_t {
	/* 'this' binding: handler
	 * [0]: target
	 * [1]: key
	 * [2]: receiver (proxy)
	 */
	if C.duk_is_number(ctx, 1) == 0 {
		C.duk_push_undefined(ctx)
		return 1
	}
	key := int(C.duk_to_int(ctx, 1))
	v, ok := getTargetValue(ctx)
	if !ok {
		C.duk_push_undefined(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_undefined(ctx)
		return 1
	}
	vv := reflect.ValueOf(v)
	switch vv.Kind() {
	case reflect.Slice, reflect.Array:
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
	default:
		C.duk_push_undefined(ctx)
		return 1
	}
	return 1
}

//export go_arr_handle_set
func go_arr_handle_set(ctx *C.duk_context) C.duk_ret_t {
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

	v, ok := getTargetValue(ctx)
	if !ok {
		C.duk_push_false(ctx)
		return 1
	}
	vv := reflect.ValueOf(v)
	switch vv.Kind() {
	case reflect.Slice, reflect.Array:
		l := vv.Len()
		if key < 0 || key >= l {
			C.duk_push_false(ctx)
			return 1
		}
		dest := vv.Index(key)
		if _, ok = goVal.(string); ok {
			goVal = fmt.Sprintf("%s", goVal) // deep copy
		}
		if err = elutils.SetValue(dest, goVal); err != nil {
			C.duk_push_false(ctx)
		} else {
			C.duk_push_true(ctx)
		}
		return 1
	default:
		C.duk_push_false(ctx)
		return 1
	}
}

func pushArrProxy(ctx *C.duk_context, v interface{}) {
	C.duk_push_array(ctx) // [ arr ]
	pushProxyGetterSetter(ctx, v, (C.duk_c_function)(C.go_arr_handle_get), (C.duk_c_function)(C.go_arr_handle_set))  // [ arr proxy ]
	C.duk_push_proxy(ctx, 0) // [ arr-proxy ]
}

//export go_map_handle_get
func go_map_handle_get(ctx *C.duk_context) C.duk_ret_t {
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
	v, ok := getTargetValue(ctx)
	if !ok {
		C.duk_push_undefined(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_undefined(ctx)
		return 1
	}
	vv := reflect.ValueOf(v)
	switch vv.Kind() {
	case reflect.Map:
		val := vv.MapIndex(reflect.ValueOf(key))
		if !val.IsValid() || !val.CanInterface() {
			C.duk_push_undefined(ctx)
			return 1
		}
		pushJsProxyValue(ctx, val.Interface())
		return 1
	default:
		C.duk_push_undefined(ctx)
		return 1
	}
	return 1
}

//export go_map_handle_set
func go_map_handle_set(ctx *C.duk_context) C.duk_ret_t {
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

	v, ok := getTargetValue(ctx)
	if !ok {
		C.duk_push_false(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_false(ctx)
		return 1
	}
	vv := reflect.ValueOf(v)
	switch vv.Kind() {
	case reflect.Map:
		mapT := vv.Type()
		elType := mapT.Elem()
		dest := elutils.MakeValue(elType)
		if _, ok = goVal.(string); ok {
			goVal = fmt.Sprintf("%s", goVal) // deep copy
		}
		if err = elutils.SetValue(dest, goVal); err == nil {
			vv.SetMapIndex(reflect.ValueOf(key), dest)
			C.duk_push_true(ctx)
		} else {
			C.duk_push_false(ctx)
		}
		return 1
	default:
		C.duk_push_undefined(ctx)
		return 1
	}
	return 1
}
func pushMapProxy(ctx *C.duk_context, v interface{}) {
	C.duk_push_object(ctx)   // [ obj ]
	pushProxyGetterSetter(ctx, v, (C.duk_c_function)(C.go_map_handle_get), (C.duk_c_function)(C.go_map_handle_set)) // [ obj proxy ]
	C.duk_push_proxy(ctx, 0) // [ obj-proxy ]
}

//export go_struct_handle_get
func go_struct_handle_get(ctx *C.duk_context) C.duk_ret_t {
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
	// fmt.Printf("-- key: %s\n", key)
	v, ok := getTargetValue(ctx)
	if !ok {
		C.duk_push_undefined(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_undefined(ctx)
		return 1
	}
	structVar := reflect.ValueOf(v)
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
	if key == "length" {
		structT := structE.Type()
		C.duk_push_int(ctx, C.duk_int_t(structT.NumField()))
		return 1
	}
	name := upperFirst(key)
	// fmt.Printf("-- name: %s\n", name)
	fv := structE.FieldByName(name)
	if !fv.IsValid() {
		// fmt.Printf("-- field %s not found\n", name)
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
		// fmt.Printf("-- method %s got\n", name)
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

//export go_struct_handle_set
func go_struct_handle_set(ctx *C.duk_context) C.duk_ret_t {
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
	// fmt.Printf("-- goVal: %v\n", goVal)
	C.duk_pop(ctx)    // [ ... ]
	if err != nil {
		C.duk_push_false(ctx)
		return 1
	}

	v, ok := getTargetValue(ctx)
	if !ok {
		C.duk_push_false(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_false(ctx)
		return 1
	}
	vv := reflect.ValueOf(v)
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
	// fmt.Printf("-- field name: %s\n", name)
	fv := structE.FieldByName(name)
	if !fv.IsValid() {
		C.duk_push_false(ctx)
		return 1
	}
	if _, ok = goVal.(string); ok {
		goVal = fmt.Sprintf("%s", goVal) // deep copy
	}
	if err = elutils.SetValue(fv, goVal); err != nil {
		C.duk_push_false(ctx)
		return 1
	}
	C.duk_push_true(ctx)
	return 1
}

//export go_struct_handle_has
func go_struct_handle_has(ctx *C.duk_context) C.duk_ret_t {
	// 'this' binding: handler
	// [0]: target
	// [1]: key
	// fmt.Printf("--- go_struct_handle_has called\n")
	if C.duk_is_string(ctx, 1) == 0 {
		C.duk_push_false(ctx)
		return 1
	}
	key := C.GoString(C.duk_get_string(ctx, 1))
	if key == "length" {
		C.duk_push_true(ctx)
		return 1
	}

	v, ok := getTargetValue(ctx)
	if !ok {
		C.duk_push_false(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_false(ctx)
		return 1
	}
	vv := reflect.ValueOf(v)
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
	// fmt.Printf("-- field name: %s\n", name)
	fv := structE.FieldByName(name)
	if !fv.IsValid() {
		C.duk_push_false(ctx)
		return 1
	}
	C.duk_push_true(ctx)
	return 1
}

//export go_struct_handle_ownKeys
func go_struct_handle_ownKeys(ctx *C.duk_context) C.duk_ret_t {
	// 'this' binding: handler
	// [0]: target
	v, ok := getTargetValue(ctx)
	if !ok {
		C.duk_push_undefined(ctx)
		return 1
	}
	if v == nil {
		C.duk_push_undefined(ctx)
		return 1
	}
	vv := reflect.ValueOf(v)
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
		C.duk_push_undefined(ctx)
		return 1
	}

	C.duk_push_array(ctx) // [ arr ]
	structT := structE.Type()
	for i:=0; i<structT.NumField(); i++ {
		name := structT.Field(i).Name
		lName := lowerFirst(name)
		pushString(ctx, lName) // [ arr key ]
		C.duk_put_prop_index(ctx, -2, C.duk_uarridx_t(i)) // [ arr ] with arr[i] = key
	}

	return 1
}

// struct
func pushStructProxy(ctx *C.duk_context, v interface{}) {
	C.duk_push_object(ctx) // [ obj ]
	pushProxyGetterSetter(ctx, v, (C.duk_c_function)(C.go_struct_handle_get), (C.duk_c_function)(C.go_struct_handle_set)) // [ obj proxy ]

	var name *C.char
	C.duk_push_c_function(ctx, (C.duk_c_function)(C.go_struct_handle_has), 2) // [ obj handler has-handler ]
	getStrPtr(&has, &name)
	C.duk_put_prop_string(ctx, -2, name) // [ obj handler ] with handler[has] = has-handler

	C.duk_push_c_function(ctx, (C.duk_c_function)(C.go_struct_handle_ownKeys), 1) // [ obj handler ownKeys-handler ]
	getStrPtr(&ownKeys, &name)
	C.duk_put_prop_string(ctx, -2, name) // [ obj handler ] with handler[ownKeys] = ownKeys-handler

	C.duk_push_c_function(ctx, (C.duk_c_function)(C.go_struct_handle_ownKeys), 1) // [ obj handler ownKeys-handler ]
	getStrPtr(&enumerate, &name)
	C.duk_put_prop_string(ctx, -2, name) // [ obj handler ] with handler[enumerate] = ownKeys-handler

	vv := reflect.ValueOf(v)
	var length int
	switch vv.Kind() {
	case reflect.Struct:
		structT := vv.Type()
		length = structT.NumField()
	case reflect.Ptr:
		structT := vv.Elem().Type()
		length = structT.NumField()
	default:
	}
	pushString(ctx, "length") // [ obj handler "length" ]
	C.duk_push_int(ctx, C.duk_int_t(length)) // [ obj handler "length" num-field ]
	C.duk_put_prop(ctx, -3) // [ obj handler ] with handler[length] = num-field

	C.duk_push_proxy(ctx, 0) // [ obj-proxy ]
}

func pushProxyGetterSetter(ctx *C.duk_context, v interface{}, getter, setter C.duk_c_function) {
	ptr := getPtrStore(uintptr(unsafe.Pointer(ctx)))
	idx := ptr.register(&v)

	// [ target ]
	C.duk_push_int(ctx, C.int(idx)) // [ target idx ]
	var name *C.char
	getStrPtr(&idxName, &name)
	C.duk_put_prop_string(ctx, -2, name)  // [ target ] with taget[name] = idx

	C.duk_push_object(ctx)  // [ target handler ]

	C.duk_push_c_function(ctx, getter, 3) // [ target handler getter ]
	getStrPtr(&get, &name)
	C.duk_put_prop_string(ctx, -2, name)  // [ target handler ] with handler[get] = getter

	C.duk_push_c_function(ctx, setter, 4) // [ target handler setter ]
	getStrPtr(&set, &name)
	C.duk_put_prop_string(ctx, -2, name)  // [ target handler ] with handler[set] = setter
}

/*
//export goProxyFuncBridge
func goProxyFuncBridge(ctx *C.duk_context) C.duk_ret_t {
	// get pointer of Golang function attached to goFuncBridge
	// [ args ... ]
	argc := int(C.duk_get_top(ctx))
	C.duk_push_current_function(ctx); // [ args ... target ]

	fnPtr, ok := getTargetValue(ctx, -1)
	C.duk_pop(ctx) // [ args ... ]
	if !ok {
		return C.DUK_RET_ERROR
	}
	if fnPtr == nil {
		return C.DUK_RET_ERROR
	}
	fnV, ok := fnPtr.(*interface{})
	if !ok {
		return C.DUK_RET_ERROR
	}
	fn := *fnV
	fnVal := reflect.ValueOf(fn)
	if fnVal.Kind() != reflect.Func {
		return C.DUK_RET_ERROR
	}
	fnType := fnVal.Type()

	// make args for Golang function
	helper := elutils.NewGolangFuncHelperDiretly(fnVal, fnType)
	getArgs := func(i int) interface{} {
		C.duk_dup(ctx, C.duk_idx_t(i - argc - 1)) // [ args ... argI ]
		defer C.duk_pop(ctx) // [ args ... ]

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
	pushJsValue(ctx, v) // [ args ... v ]
	return 1
}

//export go_func_handler_apply
func go_func_handler_apply(ctx *C.duk_context) C.duk_ret_t {
	// 'this' binding: handler
	// [0]: target
	// [1]: receiver (proxy)
	// [2]: argArray
	//
	C.duk_dup(ctx, 0) // [ ... target ]
	nargs := C.duk_get_length(ctx, 2)
	argc := int(nargs)
	for i:=0; i<argc; i++ {
		C.duk_get_prop_index(ctx, 2, C.duk_uarridx_t(i)) // [ ... target ... i-th-value ]
	}

	// call target function
	C.duk_call(ctx, C.int(nargs)) // [ ... target-result ]
	return 1
}

func pushGoFuncProxy(ctx *C.duk_context, v interface{}) {
	fnType := reflect.TypeOf(v)
	argc := fnType.NumIn()
	nargs := C.int(C.DUK_VARARGS)
	if !fnType.IsVariadic() {
		nargs = C.int(argc)
	}
	C.duk_push_c_function(ctx, (C.duk_c_function)(C.goProxyFuncBridge), nargs)  // [ target ]
	ptr := getPtrStore(uintptr(unsafe.Pointer(ctx)))
	idx := ptr.register(&v)
	C.duk_push_int(ctx, C.int(idx)) // [ target idx ]
	var name *C.char
	getStrPtr(&idxName, &name)
	C.duk_put_prop_string(ctx, -2, name)  // [ target ] with target[name] = idx
	// C.duk_push_object(ctx)     // [ target handler ]
	// C.duk_push_c_function(ctx, (C.duk_c_function)(C.go_func_handler_apply), 3) // [ target handle go_func_handler_apply ]
	// getStrPtr(&apply, &name)
	// C.duk_put_prop_string(ctx, -2, name)  // [ target handle ] with handler[apply] = go_func_handler_apply
	// C.duk_push_proxy(ctx, 0) // [ target-proxy ]
}*/

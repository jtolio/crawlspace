package reflectlang

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	ErrParser       = errors.New("parser error")
	ErrUnboundVar   = errors.New("unbound variable")
	ErrTypeMismatch = errors.New("type mismatch")
	ErrUnknownOp    = errors.New("unknown op")
	ErrRuntime      = errors.New("runtime error")
)

var (
	durationSuffixes = []string{"ns", "us", "µs", "ms", "s", "m", "h"}
)

type Environment map[string]reflect.Value

func NewEnvironment() Environment {
	return map[string]reflect.Value{
		"nil":   reflect.ValueOf(nil),
		"true":  reflect.ValueOf(true),
		"false": reflect.ValueOf(false),
	}
}

type Evaluable interface {
	Run(env Environment) ([]reflect.Value, error)
}

type position struct {
	offset, line, col int
}

type Parser struct {
	source []rune
	position
	currentChar rune
}

func NewParser(source string) *Parser {
	p := &Parser{
		source:      []rune(source),
		position:    position{offset: 0, line: 1, col: 1},
		currentChar: -1,
	}
	if len(p.source) > 0 {
		p.currentChar = p.source[0]
	}
	return p
}

func (p *Parser) advance(distance int) error {
	for i := 0; i < distance; i++ {
		if p.eof() {
			return errors.New("unexpected eof")
		}
		if p.currentChar == '\n' {
			p.line++
			p.col = 1
		} else {
			p.col++
		}
		p.offset++
		if p.offset >= len(p.source) {
			p.currentChar = -1
		} else {
			p.currentChar = p.source[p.offset]
		}
	}
	return nil
}

func (p *Parser) checkpoint() position {
	return p.position
}

func (p *Parser) restore(pos position) {
	p.position = pos
	if p.offset >= len(p.source) {
		p.currentChar = -1
	} else {
		p.currentChar = p.source[p.offset]
	}
}

func (p position) Err(errType error, messagef string, args ...interface{}) error {
	return fmt.Errorf("%w: line %d, column %d: %s",
		errType, p.line, p.col,
		fmt.Sprintf(messagef, args...))
}

func (p *Parser) sourceError(messagef string, args ...interface{}) error {
	return p.position.Err(ErrParser, messagef, args...)
}

func (p *Parser) eof() bool {
	return p.offset >= len(p.source)
}

func (p *Parser) char(lookahead int) rune {
	if p.offset+lookahead >= len(p.source) || p.offset+lookahead < 0 {
		return -1
	}
	return p.source[p.offset+lookahead]
}

func charRepr(c rune) string {
	if c == -1 {
		return "eof"
	}
	return fmt.Sprintf("%q", string(c))
}

func (p *Parser) string(width int) string {
	remaining := p.source[p.offset:]
	if len(remaining) < width {
		width = len(remaining)
	}
	return string(remaining[:width])
}

func (p *Parser) skipComment() (bool, error) {
	current := p.string(2)
	commentEnd := ""
	switch current {
	case "//":
		commentEnd = "\n"
	case "/*":
		commentEnd = "*/"
	default:
		return false, nil
	}
	if err := p.advance(2); err != nil {
		return false, err
	}
	for {
		if p.eof() {
			return true, nil
		}
		if p.string(len(commentEnd)) == commentEnd {
			return true, p.advance(len(commentEnd))
		}
		if err := p.advance(1); err != nil {
			return false, err
		}
	}
}

func (p *Parser) skipWhitespace() (bool, error) {
	if p.eof() {
		return false, nil
	}
	skipped, err := p.skipComment()
	if err != nil {
		return false, err
	}
	if skipped {
		return true, nil
	}
	switch p.currentChar {
	case ' ', '\t', '\r', '\n':
		return true, p.advance(1)
	}
	return false, nil
}

func (p *Parser) skipAllWhitespace() (bool, error) {
	anySkipped := false
	for {
		skipped, err := p.skipWhitespace()
		if err != nil {
			return false, err
		}
		if !skipped {
			return anySkipped, nil
		}
		anySkipped = true
	}
}

func isIdentifierChar(c rune) bool {
	return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c)
}

func (p *Parser) parseIdentifier() (*Ident, error) {
	cp := p.checkpoint()
	if unicode.IsDigit(p.currentChar) {
		return nil, nil
	}
	chars, err := p.parseChars(isIdentifierChar)
	if err != nil {
		return nil, err
	}
	if _, err = p.skipAllWhitespace(); err != nil {
		return nil, err
	}
	if chars == "" {
		return nil, nil
	}
	return &Ident{Name: chars, pos: cp}, nil
}

func (p *Parser) parseChars(allowed func(rune) bool) (string, error) {
	if !allowed(p.currentChar) {
		return "", nil
	}
	chars := string(p.currentChar)
	if err := p.advance(1); err != nil {
		return "", err
	}
	for allowed(p.currentChar) {
		chars += string(p.currentChar)
		if err := p.advance(1); err != nil {
			return "", err
		}
	}
	return chars, nil
}

func isUniquelyFloatingPointChar(c rune) bool {
	switch c {
	case '.', 'e', 'E', '+', '-', 'p', 'P': // floating point
		return true
	}
	return false
}

func isNumberChar(c rune) bool {
	if unicode.IsDigit(c) {
		return true
	}
	if isUniquelyFloatingPointChar(c) {
		return true
	}
	switch c {
	case 'b', 'B': // binary
		return true
	case 'o', 'O': // octal
		return true
	case 'x', 'X', 'a', 'A', 'c', 'C', 'd', 'D', 'e', 'E', 'f', 'F': // hex
		return true
	case '_': // separators
		return true
	}
	return false
}

func stringContains(val string, matcher func(rune) bool) bool {
	for _, c := range val {
		if matcher(c) {
			return true
		}
	}
	return false
}

func (p *Parser) parseDurationSuffix() (string, error) {
	for _, suffix := range durationSuffixes {
		if p.string(len(suffix)) == suffix {
			return suffix, p.advance(len(suffix))
		}
	}
	return "", nil
}

func (p *Parser) parseNumber() (Evaluable, error) {
	if !unicode.IsDigit(p.currentChar) {
		return nil, nil
	}

	num, err := p.parseChars(isNumberChar)
	if err != nil {
		return nil, err
	}

	suffix, err := p.parseDurationSuffix()
	if err != nil {
		return nil, err
	}

	if _, err = p.skipAllWhitespace(); err != nil {
		return nil, err
	}
	if num == "" {
		return nil, nil
	}

	if suffix != "" {
		dur, err := time.ParseDuration(num + suffix)
		if err != nil {
			return nil, err
		}
		return &Value{Val: reflect.ValueOf(dur)}, nil
	}

	if stringContains(num, isUniquelyFloatingPointChar) {
		val, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return nil, err
		}
		return &Value{Val: reflect.ValueOf(val)}, nil
	}
	val, err := strconv.ParseInt(num, 0, 64)
	if err != nil {
		return nil, err
	}
	return &Value{Val: reflect.ValueOf(val)}, nil
}

func (p *Parser) parseString() (Evaluable, error) {
	if p.char(0) != '"' {
		return nil, nil
	}
	if err := p.advance(1); err != nil {
		return nil, err
	}
	var val []rune
	for {
		r := p.char(0)
		if err := p.advance(1); err != nil {
			return nil, err
		}
		switch r {
		case '\\':
			r = p.char(0)
			if err := p.advance(1); err != nil {
				return nil, err
			}
			switch r {
			case '\\', '"':
				val = append(val, r)
			case 'n':
				val = append(val, '\n')
			case 't':
				val = append(val, '\t')
			default:
				return nil, p.sourceError("unexpected escape code: %s", charRepr(r))
			}
		case '"':
			_, err := p.skipAllWhitespace()
			return &Value{Val: reflect.ValueOf(string(val))}, err
		case '\n':
			return nil, p.sourceError("unexpected end of line")
		default:
			val = append(val, r)
		}
	}
}

func (p *Parser) parseLiteral() (Evaluable, error) {
	str, err := p.parseString()
	if err != nil {
		return nil, err
	}
	if str != nil {
		return str, nil
	}
	ident, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}
	if ident != nil {
		return ident, nil
	}
	return p.parseNumber()
}

func (p *Parser) parseFieldAccess(val Evaluable) (Evaluable, error) {
	cp := p.checkpoint()
	if p.char(0) != '.' {
		return nil, nil
	}
	if err := p.advance(1); err != nil {
		return nil, err
	}
	if _, err := p.skipAllWhitespace(); err != nil {
		return nil, err
	}
	field, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}
	if field == nil {
		p.restore(cp)
		return nil, nil
	}
	return &FieldAccess{Val: val, Field: field, pos: cp}, nil
}

func (p *Parser) parseArrayAccess(val Evaluable) (Evaluable, error) {
	if p.char(0) != '[' {
		return nil, nil
	}
	cp := p.checkpoint()
	if err := p.advance(1); err != nil {
		return nil, err
	}
	if _, err := p.skipAllWhitespace(); err != nil {
		return nil, err
	}
	low, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	if p.char(0) == ':' {
		if err := p.advance(1); err != nil {
			return nil, err
		}
		if _, err := p.skipAllWhitespace(); err != nil {
			return nil, err
		}
		high, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		val = &SliceAccess{
			Array: val,
			Low:   low,
			High:  high,
			pos:   cp,
		}
	} else {
		val = &ArrayAccess{
			Array: val,
			Index: low,
			pos:   cp,
		}
	}

	if p.char(0) != ']' {
		return nil, p.sourceError("expected end of array access")
	}
	if err := p.advance(1); err != nil {
		return nil, err
	}
	if _, err := p.skipAllWhitespace(); err != nil {
		return nil, err
	}

	return val, nil
}

func (p *Parser) parseArgs() ([]Evaluable, error) {
	if p.char(0) != '(' {
		return nil, nil
	}
	if err := p.advance(1); err != nil {
		return nil, err
	}
	if _, err := p.skipAllWhitespace(); err != nil {
		return nil, err
	}
	args := []Evaluable{}
	if p.char(0) == ')' {
		if err := p.advance(1); err != nil {
			return nil, err
		}
		_, err := p.skipAllWhitespace()
		return args, err
	}
	arg, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	if arg == nil {
		return nil, p.sourceError("unexpected missing argument")
	}
	args = append(args, arg)
	for {
		if p.char(0) == ')' {
			if err := p.advance(1); err != nil {
				return nil, err
			}
			_, err := p.skipAllWhitespace()
			return args, err
		}
		if p.char(0) != ',' {
			return nil, p.sourceError("unexpected character %s", charRepr(p.char(0)))
		}
		if err := p.advance(1); err != nil {
			return nil, err
		}
		if _, err := p.skipAllWhitespace(); err != nil {
			return nil, err
		}
		arg, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if arg == nil {
			return nil, p.sourceError("unexpected missing argument")
		}
		args = append(args, arg)
	}
}

func (p *Parser) parseFunctionCall(val Evaluable) (Evaluable, error) {
	cp := p.checkpoint()
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	if args == nil {
		return nil, nil
	}
	return &Call{
		Func: val,
		Args: args,
		pos:  cp,
	}, nil
}

func (p *Parser) parseModifiedSubexpression() (Evaluable, error) {
	val, err := p.parseSubexpression()
	if err != nil || val == nil {
		return val, err
	}
	for {
		if p.eof() {
			return val, nil
		}
		intermediate, err := p.parseFieldAccess(val)
		if err != nil {
			return nil, err
		}
		if intermediate != nil {
			val = intermediate
			continue
		}
		intermediate, err = p.parseArrayAccess(val)
		if err != nil {
			return nil, err
		}
		if intermediate != nil {
			val = intermediate
			continue
		}
		intermediate, err = p.parseFunctionCall(val)
		if err != nil {
			return nil, err
		}
		if intermediate != nil {
			val = intermediate
			continue
		}
		return val, nil
	}
}

func (p *Parser) parseSubexpression() (Evaluable, error) {
	cp := p.checkpoint()
	if p.char(0) != '(' {
		return p.parseLiteral()
	}
	if err := p.advance(1); err != nil {
		return nil, err
	}
	if _, err := p.skipAllWhitespace(); err != nil {
		return nil, err
	}
	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	if expr == nil {
		return nil, p.sourceError("missing subexpression")
	}
	if p.char(0) != ')' {
		return nil, p.sourceError("subexpression ended unexpectedly, found %s", charRepr(p.char(0)))
	}
	if err := p.advance(1); err != nil {
		return nil, err
	}
	_, err = p.skipAllWhitespace()
	return &Subexpression{Expr: expr, pos: cp}, err
}

func (p *Parser) parseValNegation() (Evaluable, error) {
	return p.parseModifier(
		p.parseModifiedSubexpression,
		map[string][]string{
			ModNeg:   {"-"},
			ModRef:   {"&"},
			ModDeref: {"*"},
		},
	)
}

func (p *Parser) parseMultiplicationDivision() (Evaluable, error) {
	return p.parseOperation(
		p.parseValNegation,
		map[string][]string{
			OpMul: {"*"},
			OpDiv: {"/"},
		},
	)
}

func (p *Parser) parseAdditionSubtraction() (Evaluable, error) {
	return p.parseOperation(
		p.parseMultiplicationDivision,
		map[string][]string{
			OpAdd: {"+"},
			OpSub: {"-"},
		},
	)
}

func (p *Parser) parseComparison() (Evaluable, error) {
	return p.parseOperation(
		p.parseAdditionSubtraction,
		map[string][]string{
			OpLess:         {"<"},
			OpLessEqual:    {"<="},
			OpEqual:        {"=="},
			OpNotEqual:     {"!=", "~=", "<>"},
			OpGreater:      {">"},
			OpGreaterEqual: {">="},
		},
	)
}

func (p *Parser) parseBoolNegation() (Evaluable, error) {
	return p.parseModifier(
		p.parseComparison,
		map[string][]string{
			ModNot: {"!"},
		},
	)
}

func (p *Parser) parseConjunction() (Evaluable, error) {
	return p.parseOperation(
		p.parseBoolNegation,
		map[string][]string{
			OpAnd: {"&&"},
		},
	)
}

func (p *Parser) parseDisjunction() (Evaluable, error) {
	return p.parseOperation(
		p.parseConjunction,
		map[string][]string{
			OpOr: {"||"},
		},
	)
}

func (p *Parser) parseOperation(valueParse func() (Evaluable, error),
	opMap map[string][]string) (Evaluable, error) {
	val, err := valueParse()
	if err != nil {
		return nil, err
	}
	if val == nil {
		return nil, nil
	}
	for {
		if p.eof() {
			return val, nil
		}
		cp := p.checkpoint()
		cls, rhs, err := parseOpAndRHS(p, valueParse, opMap)
		if err != nil {
			return nil, err
		}
		if cls == OpOrModNil {
			return val, nil
		}
		val = &Operation{
			Type:  OpType(cls),
			Left:  val,
			Right: rhs,
			pos:   cp,
		}
	}
}

func (p *Parser) parseModifier(valueParse func() (Evaluable, error),
	modMap map[string][]string) (Evaluable, error) {
	cp := p.checkpoint()
	cls, val, err := parseOpAndRHS(p, valueParse, modMap)
	if err != nil {
		return nil, err
	}
	if cls != OpOrModNil {
		return &Modifier{
			Type: ModType(cls),
			Val:  val,
			pos:  cp,
		}, nil
	}
	return valueParse()
}

func (p *Parser) isBoundary(char1, char2 rune) bool {
	return !isIdentifierChar(char1) || !isIdentifierChar(char2)
}

func parseOpAndRHS(p *Parser, valueParse func() (Evaluable, error),
	opMap map[string][]string) (key string, _ Evaluable, _ error) {
	cpos := p.checkpoint()
	for cls, operators := range opMap {
		for _, op := range operators {
			if strings.ToLower(p.string(len(op))) == op && p.isBoundary(p.char(len(op)-1), p.char(len(op))) {
				if err := p.advance(len(op)); err != nil {
					return OpOrModNil, nil, err
				}
				if _, err := p.skipAllWhitespace(); err != nil {
					return OpOrModNil, nil, err
				}
				rhs, err := valueParse()
				if err != nil {
					return OpOrModNil, nil, err
				}
				if rhs != nil {
					return cls, rhs, nil
				}
				p.restore(cpos)
			}
		}
	}
	return OpOrModNil, nil, nil
}

func (p *Parser) parseExpression() (Evaluable, error) {
	return p.parseDisjunction()
}

func (p *Parser) Parse() (Evaluable, error) {
	if _, err := p.skipAllWhitespace(); err != nil {
		return nil, err
	}
	val, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	if !p.eof() {
		return nil, p.sourceError("unparsed input: %q", string(p.source[p.offset:]))
	}
	if val == nil {
		return nil, p.sourceError("nothing parsed")
	}
	return val, nil
}

type Subexpression struct {
	Expr Evaluable
	pos  position
}

func (s *Subexpression) Run(env Environment) ([]reflect.Value, error) {
	return s.Expr.Run(env)
}

type Call struct {
	Func Evaluable
	Args []Evaluable
	pos  position
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

func (pos position) singleValue(results []reflect.Value, err error) (reflect.Value, error) {
	if len(results) == 0 {
		return reflect.ValueOf(nil), err
	}
	if len(results) == 1 {
		return results[0], err
	}
	return reflect.Value{}, pos.Err(ErrRuntime, "multivalue result used in single value location")
}

type lowerFunc struct {
	Env  Environment
	Func func([]reflect.Value) ([]reflect.Value, error)
}

func LowerFunc(env Environment, fn func([]reflect.Value) ([]reflect.Value, error)) reflect.Value {
	return reflect.ValueOf(lowerFunc{
		Env:  env,
		Func: fn,
	})
}

func (c *Call) Run(env Environment) ([]reflect.Value, error) {
	fn, err := c.pos.singleValue(c.Func.Run(env))
	if err != nil {
		return nil, err
	}

	args := make([]reflect.Value, 0, len(c.Args))
	for i := range c.Args {
		result, err := c.Args[i].Run(env)
		if err != nil {
			return nil, err
		}
		if i == 0 && len(c.Args) == 1 {
			args = result
			break
		}
		arg, err := c.pos.singleValue(result, nil)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}

	if fn.Kind() == reflect.Struct {
		if lf, ok := fn.Interface().(lowerFunc); ok &&
			reflect.ValueOf(lf.Env).Pointer() == reflect.ValueOf(env).Pointer() {
			return lf.Func(args)
		}
	}

	return fn.Call(args), nil
}

type lowerStruct struct {
	Env   Environment
	Field func(name string) ([]reflect.Value, error)
}

func LowerStruct(env Environment, sub Environment) reflect.Value {
	return reflect.ValueOf(lowerStruct{
		Env: env,
		Field: func(name string) ([]reflect.Value, error) {
			if v, ok := sub[name]; ok {
				return []reflect.Value{v}, nil
			}
			return nil, fmt.Errorf("%w: field %q in LowerStruct not found", ErrTypeMismatch, name)
		}})
}

type FieldAccess struct {
	Val   Evaluable
	Field *Ident
	pos   position
}

func (a *FieldAccess) Run(env Environment) ([]reflect.Value, error) {
	v, err := a.pos.singleValue(a.Val.Run(env))
	if err != nil {
		return nil, err
	}

	if v.Kind() == reflect.Struct {
		if ls, ok := v.Interface().(lowerStruct); ok &&
			reflect.ValueOf(ls.Env).Pointer() == reflect.ValueOf(env).Pointer() {
			return ls.Field(a.Field.Name)
		}
	}

	tryAccess := func(v reflect.Value) ([]reflect.Value, bool) {
		method := v.MethodByName(a.Field.Name)
		if method != (reflect.Value{}) {
			return []reflect.Value{method}, true
		}
		if v.Kind() == reflect.Struct {
			return []reflect.Value{v.FieldByName(a.Field.Name)}, true
		}
		return nil, false
	}

	if rv, found := tryAccess(v); found {
		return rv, nil
	}

	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if rv, found := tryAccess(v.Elem()); found {
			return rv, nil
		}
	}

	return nil, a.pos.Err(ErrTypeMismatch, "tried to access field %q on value %#v, %v", a.Field.Name, v, v.Kind())
}

type ArrayAccess struct {
	Array Evaluable
	Index Evaluable
	pos   position
}

func (a *ArrayAccess) Run(env Environment) ([]reflect.Value, error) {
	v, err := a.pos.singleValue(a.Array.Run(env))
	if err != nil {
		return nil, err
	}
	index, err := a.pos.singleValue(a.Index.Run(env))
	if err != nil {
		return nil, err
	}

	switch v.Kind() {
	case reflect.Array, reflect.Slice, reflect.String:
		if !index.CanInt() {
			return nil, a.pos.Err(ErrTypeMismatch, "index %q is not an int", index)
		}
		return []reflect.Value{v.Index(int(index.Int()))}, nil
	case reflect.Map:
		return []reflect.Value{v.MapIndex(index)}, nil
	}
	return nil, a.pos.Err(ErrTypeMismatch, "tried to access index %q on value %#v (%v)", index, v, v.Kind())
}

type SliceAccess struct {
	Array Evaluable
	Low   Evaluable
	High  Evaluable
	pos   position
}

func (a *SliceAccess) Run(env Environment) ([]reflect.Value, error) {
	v, err := a.pos.singleValue(a.Array.Run(env))
	if err != nil {
		return nil, err
	}
	l, err := a.pos.singleValue(a.Low.Run(env))
	if err != nil {
		return nil, err
	}
	h, err := a.pos.singleValue(a.Low.Run(env))
	if err != nil {
		return nil, err
	}
	switch v.Kind() {
	default:
		return nil, a.pos.Err(ErrTypeMismatch, "tried to slice value %q", v)
	case reflect.Array, reflect.Slice, reflect.String:
	}
	if !l.CanInt() {
		return nil, a.pos.Err(ErrTypeMismatch, "slice index %q not an int", l)
	}
	if !h.CanInt() {
		return nil, a.pos.Err(ErrTypeMismatch, "slice index %q not an int", h)
	}
	return []reflect.Value{v.Slice(int(l.Int()), int(h.Int()))}, nil
}

type Operation struct {
	Type  OpType
	Left  Evaluable
	Right Evaluable
	pos   position
}

func (o *Operation) Run(env Environment) ([]reflect.Value, error) {
	left, err := o.pos.singleValue(o.Left.Run(env))
	if err != nil {
		return nil, err
	}
	switch o.Type {
	case OpEqual, OpNotEqual:
		right, err := o.pos.singleValue(o.Right.Run(env))
		if err != nil {
			return nil, err
		}
		rv := left.Equal(right)
		if o.Type == OpNotEqual {
			rv = !rv
		}
		return []reflect.Value{reflect.ValueOf(rv)}, nil
	case OpAnd:
		if !left.Bool() {
			// short circuit eval
			return []reflect.Value{left}, nil
		}
		rv, err := o.pos.singleValue(o.Right.Run(env))
		if err != nil {
			return nil, err
		}
		return []reflect.Value{rv}, nil
	case OpOr:
		if left.Bool() {
			// short circuit eval
			return []reflect.Value{left}, nil
		}
		rv, err := o.pos.singleValue(o.Right.Run(env))
		if err != nil {
			return nil, err
		}
		return []reflect.Value{rv}, nil
	case OpMul:
	case OpDiv:
	case OpAdd:
	case OpSub:
	case OpLess:
	case OpLessEqual:
	case OpGreater:
	case OpGreaterEqual:
	}
	return nil, o.pos.Err(ErrUnknownOp, "%q", o.Type)
}

type OpType = string

const (
	OpOrModNil            = ""
	OpMul          OpType = "*"
	OpDiv          OpType = "/"
	OpAdd          OpType = "+"
	OpSub          OpType = "-"
	OpLess         OpType = "<"
	OpLessEqual    OpType = "<="
	OpEqual        OpType = "=="
	OpNotEqual     OpType = "!="
	OpGreater      OpType = ">"
	OpGreaterEqual OpType = ">="
	OpAnd          OpType = "&&"
	OpOr           OpType = "||"
)

type Modifier struct {
	Type ModType
	Val  Evaluable
	pos  position
}

func (m *Modifier) Run(env Environment) ([]reflect.Value, error) {
	val, err := m.pos.singleValue(m.Val.Run(env))
	if err != nil {
		return nil, err
	}

	switch m.Type {
	case ModNeg:
	case ModNot:
		if val.Kind() == reflect.Bool {
			return []reflect.Value{reflect.ValueOf(!val.Bool())}, nil
		}
	case ModRef:
		return []reflect.Value{val.Addr()}, nil
	case ModDeref:
		return []reflect.Value{val.Elem()}, nil
	}
	return nil, m.pos.Err(ErrUnknownOp, "%q", m.Type)
}

type ModType = string

const (
	ModNeg   ModType = "-"
	ModNot   ModType = "!"
	ModRef   ModType = "&"
	ModDeref ModType = "*"
)

type Ident struct {
	Name string
	pos  position
}

func (i *Ident) Run(env Environment) ([]reflect.Value, error) {
	if v, ok := env[i.Name]; ok {
		return []reflect.Value{v}, nil
	}
	return nil, fmt.Errorf("%w: %#v", ErrUnboundVar, i.Name)
}

type Value struct {
	Val reflect.Value
}

func (v *Value) Run(env Environment) ([]reflect.Value, error) {
	return []reflect.Value{v.Val}, nil
}

func Parse(expression string) (Evaluable, error) {
	return NewParser(expression).Parse()
}

func Eval(expression string, env Environment) (_ []reflect.Value, err error) {
	val, err := Parse(expression)
	if err != nil {
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			if re, ok := r.(error); ok {
				err = fmt.Errorf("panic: %w", re)
				return
			}
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return val.Run(env)
}

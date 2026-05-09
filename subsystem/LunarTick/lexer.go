package main

import (
	"strings"
	"unicode"
)

// TokenType 词法单元类型
type TokenType int

const (
	TokenUnknown TokenType = iota
	TokenDirective    // @开头的指令
	TokenVariable     // #开头的变量
	TokenPointer      // *开头的指针
	TokenString       // 字符串字面量
	TokenNumber       // 数字字面量
	TokenIdentifier   // 标识符
	TokenOperator     // 运算符
	TokenComment      // 注释
	TokenEOL          // 行结束
)

// Token 词法单元
type Token struct {
	Type  TokenType
	Value string
	Pos   int
}

// Lexer 词法分析器
type Lexer struct {
	input string
	pos   int
}

// NewLexer 创建词法分析器
func NewLexer(input string) *Lexer {
	return &Lexer{
		input: input,
		pos:   0,
	}
}

// Tokenize 对整行进行词法分析
func (l *Lexer) Tokenize() []Token {
	var tokens []Token
	
	for l.pos < len(l.input) {
		l.skipWhitespace()
		
		if l.pos >= len(l.input) {
			break
		}
		
		switch {
		case l.input[l.pos] == '@':
			tokens = append(tokens, l.readDirective())
		case l.input[l.pos] == '#':
			tokens = append(tokens, l.readVariable())
		case l.input[l.pos] == '*':
			tokens = append(tokens, l.readPointer())
		case l.input[l.pos] == '"' || l.input[l.pos] == '\'':
			tokens = append(tokens, l.readString())
		case l.input[l.pos] == '/' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '/':
			tokens = append(tokens, l.readComment())
			return tokens // 注释后不再处理
		case unicode.IsDigit(rune(l.input[l.pos])):
			tokens = append(tokens, l.readNumber())
		case unicode.IsLetter(rune(l.input[l.pos])):
			tokens = append(tokens, l.readIdentifier())
		default:
			tokens = append(tokens, l.readOperator())
		}
	}
	
	return tokens
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && unicode.IsSpace(rune(l.input[l.pos])) {
		l.pos++
	}
}

func (l *Lexer) readDirective() Token {
	start := l.pos
	l.pos++ // 跳过 @
	
	for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos]))) {
		l.pos++
	}
	
	return Token{
		Type:  TokenDirective,
		Value: l.input[start:l.pos],
		Pos:   start,
	}
}

func (l *Lexer) readVariable() Token {
	start := l.pos
	l.pos++ // 跳过 #
	
	for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '_') {
		l.pos++
	}
	
	return Token{
		Type:  TokenVariable,
		Value: l.input[start:l.pos],
		Pos:   start,
	}
}

func (l *Lexer) readPointer() Token {
	start := l.pos
	l.pos++ // 跳过 *
	
	for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '_') {
		l.pos++
	}
	
	return Token{
		Type:  TokenPointer,
		Value: l.input[start:l.pos],
		Pos:   start,
	}
}

func (l *Lexer) readString() Token {
	start := l.pos
	quote := l.input[l.pos]
	l.pos++ // 跳过引号
	
	for l.pos < len(l.input) {
		if l.input[l.pos] == quote {
			l.pos++
			break
		}
		if l.input[l.pos] == '\\' && l.pos+1 < len(l.input) {
			l.pos++ // 跳过转义字符
		}
		l.pos++
	}
	
	return Token{
		Type:  TokenString,
		Value: l.input[start:l.pos],
		Pos:   start,
	}
}

func (l *Lexer) readComment() Token {
	start := l.pos
	l.pos = len(l.input) // 跳过整行
	
	return Token{
		Type:  TokenComment,
		Value: l.input[start:],
		Pos:   start,
	}
}

func (l *Lexer) readNumber() Token {
	start := l.pos
	
	for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
		l.pos++
	}
	
	return Token{
		Type:  TokenNumber,
		Value: l.input[start:l.pos],
		Pos:   start,
	}
}

func (l *Lexer) readIdentifier() Token {
	start := l.pos
	
	for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '_') {
		l.pos++
	}
	
	return Token{
		Type:  TokenIdentifier,
		Value: l.input[start:l.pos],
		Pos:   start,
	}
}

func (l *Lexer) readOperator() Token {
	start := l.pos
	l.pos++
	
	return Token{
		Type:  TokenOperator,
		Value: l.input[start:l.pos],
		Pos:   start,
	}
}

// ParseLine 解析一行代码，返回指令类型和参数
func ParseLine(line string) (string, []string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "//") {
		return "", nil
	}
	
	lexer := NewLexer(line)
	tokens := lexer.Tokenize()
	
	if len(tokens) == 0 {
		return "", nil
	}
	
	var directive string
	var args []string
	
	for _, token := range tokens {
		switch token.Type {
		case TokenDirective:
			directive = token.Value
		case TokenString:
			// 去掉引号
			s := token.Value
			if len(s) >= 2 {
				s = s[1 : len(s)-1]
			}
			args = append(args, s)
		case TokenVariable, TokenPointer, TokenNumber, TokenIdentifier:
			args = append(args, token.Value)
		case TokenOperator:
		if len(args) > 0 {
			prev := args[len(args)-1]
			newVal := prev + token.Value
			if newVal == ">=" || newVal == "<=" || newVal == "==" || newVal == "!=" {
				args[len(args)-1] = newVal
			} else {
				args = append(args, token.Value)
			}
		} else {
			args = append(args, token.Value)
		}
		}
	}
	
	return directive, args
}

// SplitArgs 分割参数（处理引号）
func SplitArgs(input string) []string {
	var args []string
	var current strings.Builder
	inQuotes := false
	quoteChar := rune(0)
	
	for _, r := range input {
		switch {
		case r == '"' || r == '\'':
			if inQuotes && r == quoteChar {
				inQuotes = false
				quoteChar = 0
			} else if !inQuotes {
				inQuotes = true
				quoteChar = r
			} else {
				current.WriteRune(r)
			}
		case unicode.IsSpace(r) && !inQuotes:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	
	return args
}

// UnquoteString 去掉字符串的引号
func UnquoteString(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

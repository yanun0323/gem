package gem

import "strings"

// quote 封裝了所有與引號相關的操作
type quote struct {
	char rune
}

// newQuote 創建一個新的 Quote 實例
func newQuote(char rune) *quote {
	return &quote{char: char}
}

// wrapMap 返回引號字符的字符串表示
func (q *quote) wrapMap() [2]rune {
	var quoteMap = map[rune][2]rune{
		'[': {'[', ']'},
		'"': {'"', '"'},
		'`': {'`', '`'},
		0:   {'`', '`'},
	}
	if quotePair, exist := quoteMap[q.char]; exist {
		return quotePair
	}
	return [2]rune{q.char, q.char}
}

// Wrap 將字符串用引號包裹
func (q *quote) Wrap(s string) string {
	quotePair := q.wrapMap()
	return string(quotePair[0]) + s + string(quotePair[1])
}

// Unwrap 移除字符串兩端的引號
func (q *quote) Unwrap(s string) string {
	quotePair := q.wrapMap()
	for _, char := range quotePair {
		s = strings.Trim(s, string(char))
	}
	return s
}

// RegexPattern 返回用於正則表達式的引號模式
func (q *quote) RegexPattern() string {
	quotePair := q.wrapMap()
	return string(quotePair[0]) + `(\w+)` + string(quotePair[1])
}

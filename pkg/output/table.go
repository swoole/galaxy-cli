package output

import (
	"errors"
	"fmt"
	"reflect"
)

// PlainTable formats a slice of structs as an unbordered text table. Exported
// fields become columns and the table struct tag can rename or hide a column.
func PlainTable(slice interface{}) string {
	columns, widths, rows, err := parse(slice)
	if err != nil {
		return err.Error()
	}
	if len(rows) == 0 {
		return ""
	}

	var result string
	for i, width := range widths {
		result += " " + columns[i] + repeat(width-StringLength([]rune(columns[i]))+1, ' ')
	}
	result += "\n"

	for _, row := range rows {
		for i, width := range widths {
			result += " " + row[i] + repeat(width-StringLength([]rune(row[i]))+1, ' ')
		}
		result += "\n"
	}
	return result
}

func parse(slice interface{}) (columns []string, widths []int, rows [][]string, err error) {
	value := reflect.ValueOf(slice)
	if value.Kind() != reflect.Slice {
		return nil, nil, nil, errors.New("warning: table: parameter must be a slice")
	}

	for rowIndex := 0; rowIndex < value.Len(); rowIndex++ {
		itemValue := value.Index(rowIndex)
		itemType := itemValue.Type()
		if itemValue.Kind() == reflect.Ptr {
			itemValue = itemValue.Elem()
			itemType = itemType.Elem()
		}
		if itemValue.Kind() != reflect.Struct {
			return nil, nil, nil, errors.New("warning: table: slice items must be structs")
		}

		var row []string
		for fieldIndex := 0; fieldIndex < itemValue.NumField(); fieldIndex++ {
			field := itemType.Field(fieldIndex)
			if field.PkgPath != "" {
				continue
			}
			column := field.Tag.Get("table")
			if column == "-" {
				continue
			}
			if column == "" {
				column = field.Name
			}
			content := fmt.Sprintf("%+v", itemValue.Field(fieldIndex).Interface())
			if rowIndex == 0 {
				columns = append(columns, column)
				widths = append(widths, StringLength([]rune(column)))
			}
			columnIndex := len(row)
			if contentWidth := StringLength([]rune(content)); contentWidth > widths[columnIndex] {
				widths[columnIndex] = contentWidth
			}
			row = append(row, content)
		}
		rows = append(rows, row)
	}
	return columns, widths, rows, nil
}

func repeat(count int, char rune) string {
	runes := make([]rune, count)
	for i := range runes {
		runes[i] = char
	}
	return string(runes)
}

// StringLength returns the display width of a string, counting common CJK
// characters as two terminal cells.
func StringLength(runes []rune) int {
	type interval struct {
		from rune
		to   rune
	}
	wide := []interval{
		{0x2E80, 0x9FD0},
		{0xAC00, 0xD7A3},
		{0xF900, 0xFACE},
		{0xFE00, 0xFE6C},
		{0xFF00, 0xFF60},
		{0x20000, 0x2FA1D},
	}
	length := len(runes)
	for _, current := range runes {
		for _, item := range wide {
			if current >= item.from && current <= item.to {
				length++
				break
			}
		}
	}
	return length
}

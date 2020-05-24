package main

import (
    "github.com/gdamore/tcell"
)

func Clear(scr tcell.Screen, x, y, h, w int, c rune, style tcell.Style) {
    for j:=y; j<y+h; j++ {
        for i:=x; i<x+w; i++ {
            scr.SetContent(i, j, c, nil, style)
        }
    }
}

func DrawLines(scr tcell.Screen, x, y int, style tcell.Style, lines []string) {
    for j, line := range lines {
        for i, c := range line {
            scr.SetContent(x+i, y+j, c, nil, style)
        }
    }
}

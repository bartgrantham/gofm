package main

import (
    "bufio"
    "errors"
    "io"
    "strconv"
    "strings"
)

// See: figfont.txt

type FIGfont struct {
    Name       string
    Height     int
    hardblank  byte
    baseline   int
    maxlen     int
    oldlayout  int
    comments   int
    direction  int
    layout     int
    codetags   int
    chars      map[rune][]string
}

// Full layout
const (
    horizontal_smush_1    = 1<<iota
    horizontal_smush_2
    horizontal_smush_3
    horizontal_smush_4
    horizontal_smush_5
    horizontal_smush_6
    horizontal_fit
    horizontal_smush   // overrides horizontal_fit
    vertical_smush_1
    vertical_smush_2
    vertical_smush_3
    vertical_smush_4
    vertical_smush_5
    vertical_fit
    vertical_smush
)

// Old layout
const (
    full_width_layout  = -1
    old_horizontal_fit     = 0
//    horizontal_smush_1    = 1<<iota
//    horizontal_smush_2
//    horizontal_smush_3
//    horizontal_smush_4
//    horizontal_smush_5
//    horizontal_smush_6
)

var ErrInvalidFont  = errors.New("invalid FIGfont")
var ErrParse        = errors.New("couldn't parse FIGfont")

var charorder string = ` !"#$%&'()*+,-./` + `0123456789:;<=>?` + `@ABCDEFGHIJKLMNO` +
                       `PQRSTUVWXYZ[\]^_` + "`abcdefghijklmno" + "pqrstuvwxyz{|}~" +
                       "\xc4\xd6\xdc\xe4\xf6\xfc\xdf"

func (f *FIGfont) String() string {
    return f.Name
}

func NewFIGfont(r io.Reader) (*FIGfont, error) {
    var err error
    var lines, header []string
    var params []int
    var s string
    var i int

    scanner := bufio.NewScanner(r)
    for scanner.Scan() {
        lines = append(lines, scanner.Text())
    }

    header = strings.Fields(lines[0])
    if header[0][0:5] != "flf2a" {
        return nil, ErrParse
    }

    for _, s = range header[1:] {
        if i, err = strconv.Atoi(s); err != nil {
            return nil, err
        }
        params = append(params, i)
    }

    f := FIGfont{}
    f.hardblank = header[0][5]

    if len(params) > 0 {
        f.Height = params[0]
    }
    if len(params) > 1 {
        f.baseline = params[1]
    }
    if len(params) > 2 {
        f.maxlen = params[2]
    }
    if len(params) > 3 {
        f.oldlayout = params[3]
    }
    if len(params) > 4 {
        f.comments = params[4]
    }
    if len(params) > 5 {
        f.direction = params[5]
    }
    if len(params) > 6 {
        f.layout = params[6]
    }
    if len(params) > 7 {
        f.codetags = params[7]
    }

    f.chars = map[rune][]string{}
    var idx int
    var line, endmark string
    for i, c := range charorder {
        idx = 1 + f.comments + (i*f.Height)
        endmark = lines[idx][ len(lines[idx])-1: ]
        for j:=0; j<f.Height; j++ {
            line = lines[idx+j]
            f.chars[c] = append(f.chars[c], strings.TrimRight(line, endmark))
        }
    }
    return &f, nil
}

// very stupid renderer that does _not_ respect FIGlet's rules
func (f *FIGfont) Render(s string) []string {
    var out, fig []string
    var line string
    var i int
    var c rune
    var ok bool

    for i=0; i<f.Height; i++ {
        out = append(out, "")
    }

    for _, c = range s {
        if c == 0 {
            return out
        }
        if fig, ok = f.chars[c]; !ok {
            continue
        }
        for i=0; i<f.Height; i++ {
            line = strings.Replace(fig[i], string([]byte{f.hardblank}), " ", -1)
            out[i] += line
        }
    }
    // TODO: strip ' ' off the right

    return out
}


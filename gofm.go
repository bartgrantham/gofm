package main

import (
    "fmt"
    "io"
    "os"
    "time"

    "github.com/gdamore/tcell"

    "periph.io/x/conn/v3/gpio"

    "periph.io/x/conn/v3/i2c"
    "periph.io/x/conn/v3/i2c/i2creg"
    "periph.io/x/conn/v3/pin/pinreg"

    "periph.io/x/host/v3"
    "periph.io/x/host/v3/rpi"
)

var i2c_addr = 0x10

func main() {
    var err error
    var r io.Reader
    var scr tcell.Screen
    var big, medium *FIGfont

    if scr, err = tcell.NewScreen(); err != nil {
        fmt.Println("couldn't open screen:", err)
        return
    }
    if err = scr.Init(); err != nil {
        fmt.Println("couldn't init screen:", err)
    }
    defer scr.Fini()
    scr.Clear()

    if r, err = os.Open("univers.flf"); err != nil {
        fmt.Println(err)
        return
    }
    if big, err = NewFIGfont(r); err != nil {
        fmt.Println(err)
        return
    }
    if r, err = os.Open("nancyj-improved.flf"); err != nil {
        fmt.Println(err)
        return
    }
    if medium, err = NewFIGfont(r); err != nil {
        fmt.Println(err)
        return
    }

    busname := "I2C1"
    if _, err = host.Init(); err != nil {
        fmt.Println("couldn't initialize peripherals:", err)
        return
    }

    bus, err := i2creg.Open(busname)
    if err != nil {
        fmt.Println("couldn't initialize i2c bus:", err)
        return
    }

    p, _ := bus.(i2c.Pins)
    _, scl_pin := pinreg.Position(p.SCL())
    _, sda_pin := pinreg.Position(p.SDA())
    fmt.Printf("Using i2c \"%s\"\n    %s : pin %d\n    %s : pin %d\n",
        bus, p.SCL(), scl_pin, p.SDA(), sda_pin)

    // initialize 
    fmt.Println("initializing...")
    // reset low
    rpi.P1_16.Out(gpio.Low)  // GPIO23 == RPI16
    time.Sleep(100 * time.Millisecond)
    rpi.P1_16.Out(gpio.High)
    time.Sleep(100 * time.Millisecond)

    s, _ := NewSi4703(bus, uint16(i2c_addr))
    fmt.Println("at power on", s.Reg)

    var tmp uint16

    // turn on oscillator
    s.SetOsc(true)
    fmt.Println("osc on", s.Reg)


    // enable radio and turn off mute
    fmt.Println("enable radio 0 :", s.Reg)
    s.Set(POWERCFG, 0x4001)  // DMUTE | ENABLE
    fmt.Println("enable radio 1 :", s.Reg)
    time.Sleep(100 * time.Millisecond)
    s.Read()
    fmt.Println("enable radio 2 :", s.Reg)

/*
    fmt.Println("enable radio 0 :", s.Reg)
    s.Enable()
    fmt.Println("enable radio 1a :", s.Reg)
    s.Mute(false)
    fmt.Println("enable radio 1b :", s.Reg)
    time.Sleep(500 * time.Millisecond)  // wait for crystal powerup
    s.Read()
    fmt.Println("enable radio 2 :", s.Reg)
*/

//    s.RDS(true)

    tmp = s.Reg[SYSCONFIG1]
    tmp |= uint16(1<<12)       // enable RDS
    s.Set(SYSCONFIG1, tmp)
    fmt.Println("enable RDS/set volume 0 :", s.Reg)
//    s.Volume(0)
    s.Set(SYSCONFIG2, 0)       // set volume to lowest
    s.Set(SYSCONFIG3, 0x0100)  // set extended volume
    fmt.Println("enable RDS/set volume 1 :", s.Reg)
    time.Sleep(100 * time.Millisecond)
    s.Read()
    fmt.Println("enable RDS/set volume 2 :", s.Reg)

    channel := float64(88.5)
    s.SetChannel(channel)  // 88.5  103.7  107.7
    // 104.9 has all the accents!

//Scan(s)
//os.Exit(0)

    s.Set(SYSCONFIG2, 15)  // set volume to max

    black := tcell.Color(int32(232))
    white := tcell.Color(int32(255))

    freq_style := tcell.StyleDefault
    freq_style = freq_style.Foreground(white)
    freq_style = freq_style.Background(black)
    freq_style = freq_style.Bold(true)

    call_style := tcell.StyleDefault
//    fg := tcell.Color(int32(tcell.ColorTeal))
//    bg := tcell.Color(int32(tcell.ColorBlack))
//    style = style.Foreground(fg)
//    style = style.Background(bg)

    // print rssi, checksum X, ....
    var rssi, x_tmp int
    var rdsr, traffic rune
    var msg, stereo string
    rds := RDS{}

    scr.Clear()
    scr.EnableMouse()
    event := make(chan tcell.Event, 1)
    go func(){
        for {
            event<-scr.PollEvent()
        }
    }()

    w, _ := scr.Size()
    var e tcell.Event
    evtloop:
    for {
        select {
            case e = <-event:
                switch e := e.(type) {
                    case *tcell.EventKey:
                        switch e.Key() {
                            case tcell.KeyCtrlC: break evtloop
                            case tcell.KeyUp:
                                channel += .2
                                if channel > 107.9 {
                                    channel = 87.5
                                }
                                s.SetChannel(channel)
                                rds = RDS{}
                            case tcell.KeyDown:
                                channel -= .2
                                if channel < 87.5 {
                                    channel = 107.9
                                }
                                s.SetChannel(channel)
                                rds = RDS{}
                        }
                }
            case <-s.Update:
                // 0a : STC tuning is complete, SF/BL indicates seek band rollover, ST indicates stereo
                //     RDSR indicates RDS data ready, RSS[7:0] indicate RSSI for the current channel
                //     15 is RDSR, 13 is ?valuesfbl?
                // 0b : READCHAN[9:0] is the current channel,  15 14 13 12 11 10
                if s.Reg[STATUSRSSI] & 0x8000 == 0x8000 {
                    rdsr = 'X'
                    rds.Update(s.Reg[RDSA], s.Reg[RDSB], s.Reg[RDSC], s.Reg[RDSD])
                } else {
                    rdsr = ' '
                }
                if s.Reg[STATUSRSSI] & 0x0010 == 0x0010 {
                    stereo = "Stereo"
                } else {
                    stereo = "Mono  "
                }
                if rds.TrafficProgram {
                    traffic = 'T'
                } else {
                    traffic = ' '
                }

                rssi = int(s.Reg[STATUSRSSI] & 0xff)
                actual := float64(87.5 + (.2 * float64(s.Reg[READCHAN] & 0x1ff)))
                msg = fmt.Sprintf("%.1f (%.1f)  %.4s (%s) : %3.d  %s  %c  %c  : %.8s : %s\n", channel, actual, rds.CallSign[:], PT_NA[rds.ProgramType], rssi, stereo, rdsr, traffic, rds.ProgramService, rds.Radiotext)
                _ = msg
                FREQ := big.Render(fmt.Sprintf("%.1f", channel))
                CALL := medium.Render(string(rds.CallSign[:]))
                PROG := medium.Render(rds.Radiotext)

                x_tmp = (w - 60) / 2
                Clear(scr, x_tmp, 4, big.Height+1, 60, ' ', freq_style)
                x_tmp = (w - len(FREQ[0])) / 2
                DrawLines(scr, x_tmp, 2, freq_style, FREQ)

                x_tmp = (w - 50) / 2
                Clear(scr, x_tmp, 18, medium.Height, 50, ' ', call_style)
                x_tmp = (w - len(CALL[0])) / 2
                DrawLines(scr, x_tmp, 15, call_style, CALL)

                x_tmp = (w - len(PT_NA[rds.ProgramType])) / 2
                DrawLines(scr, x_tmp, 22, call_style, []string{PT_NA[rds.ProgramType]})

                Clear(scr, 0, 24, medium.Height, w, ' ', call_style)
                DrawLines(scr, 0, 24, call_style, PROG)
                scr.Show()

                rt := "- - - = = =  "+ rds.Radiotext +"  = = = - - -"
                rt_x := (w - len(rt)) / 2
                Clear(scr, 0, 33, 1, w, ' ', call_style)
                DrawLines(scr, rt_x, 33, call_style, []string{rt})

                x_tmp := (w - len(rds.ProgramService)) / 2
                DrawLines(scr, x_tmp, 34, call_style, []string{"("+ rds.ProgramService +")"})
        }
    }
}

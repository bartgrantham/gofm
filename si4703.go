package main

import (
    "errors"
    "fmt"
    "log"
    "io"
    "os"
    "sync"
    "time"

    "periph.io/x/periph/conn/i2c"
)

var ErrInvalidReg  = errors.New("invalid register")
var ErrInvalidFreq = errors.New("invalid frequency")
var ErrTimeout     = errors.New("timeout")

type Si4703 struct {
    sync.Mutex
    device   i2c.Dev
    Polling  bool
    Rate     time.Duration
    Reg      [16]uint16
    Update   chan struct{}
}

const (
    // registers 0..1 are read-only
    DEVICEID    = iota
    CHIPID
    // registers 2..7 are read-write
    POWERCFG
    CHANNEL
    SYSCONFIG1
    SYSCONFIG2
    SYSCONFIG3
    OSCILLATOR

    // no registers 8, 9 ; registers a..f are read-only
    _
    _
    STATUSRSSI
    READCHAN
    RDSA
    RDSB
    RDSC
    RDSD
)

//const (
//    ENABLE   uint16 = 0x0001  // POWERCFG
//    DISABLE         = 0x0040  // POWERCFG
//    MUTE            = 0x4000  // POWERCFG
//)

// current station, current volume, tune status, rssi, current rds?
func (s *Si4703) String() string {
    return "Si4703"
}

/*
From AN230:
> When using the polling method, it is best not to poll continuously.
> The data will appear in intervals of ~88 ms and the RDSR indicator will be
> available for at least 40 ms, so a polling rate of 40 ms or less should be sufficient.
*/
func NewSi4703(bus i2c.BusCloser, addr uint16) (*Si4703, error) {
    s := Si4703{
        device: i2c.Dev{Bus: bus, Addr: addr},
        Polling: true,
        Rate: 40 * time.Millisecond,
        Update: make(chan struct{}, 1),
    }

    go func() {
        next := time.Now()
        for {
            next = next.Add(s.Rate)
            time.Sleep(time.Until(next))
            if s.Polling {
                s.Read()
                select {
                    case s.Update<-struct{}{}:
                    default:
                }
            }
        }
    }()

    s.Read()
    return &s, nil
}

func (s *Si4703) Read() error {
    buf := make([]byte, 32)
    s.Lock()
    defer s.Unlock()
    if err := s.device.Tx(nil, buf); err != nil {
        return err
    }
    for i:=0; i<16; i++ {
        // (i+10) % 16 == 10, 11, 12, 13, 14, 15, 0, 1....
        // i*2   == 20, 22, 24, 26, 28, 30, 0, 2...
        // i*2+1 == 21, 23, 25, 27, 29, 31, 1, 3...
        s.Reg[ (i+10) % 16 ] = uint16(buf[i*2])*256 + uint16(buf[i*2+1])
    }
    return nil
}

func (s *Si4703) Set(reg int, val uint16) error {
    var n int
    var err error

    // big-endian: high byte comes first
    idxh := (reg-2) * 2
    idxl := idxh+1
    varh := byte(val >> 8)
    varl := byte(val & 0xff)

    if err = s.Read(); err != nil {
        return err
    }

    buf := make([]byte, 12)
    // just long enough for us to grab the current registers,
    // update with our new value, and write back to the device
    s.Lock()

    // could be a loop, but simpler to unroll
    buf[0]  = byte(s.Reg[2] >> 8)
    buf[1]  = byte(s.Reg[2] & 0xff)
    buf[2]  = byte(s.Reg[3] >> 8)
    buf[3]  = byte(s.Reg[3] & 0xff)
    buf[4]  = byte(s.Reg[4] >> 8)
    buf[5]  = byte(s.Reg[4] & 0xff)
    buf[6]  = byte(s.Reg[5] >> 8)
    buf[7]  = byte(s.Reg[5] & 0xff)
    buf[8]  = byte(s.Reg[6] >> 8)
    buf[9]  = byte(s.Reg[6] & 0xff)
    buf[10] = byte(s.Reg[7] >> 8)
    buf[11] = byte(s.Reg[7] & 0xff)

    // new value
    buf[idxh] = varh
    buf[idxl] = varl

    // write to device
    n, err = s.device.Write(buf)
    s.Unlock()
    if err != nil {
        return err
    }
    if n != 12 {
        return io.ErrShortWrite
    }
    // update our cached state
    err = s.Read()
    if err != nil {
        return err
    }
    return nil
}

/*
Changing the channel, AFAICT:

1. mask off the old channel bits (lower 17 bits)
2. set channel | (1<<15)  (TUNE: top bit of 2nd LSB)
3. send register update
4. wait for s.Reg[STATUSRSSI] & (1<<14) != 0  // 14 == STC
5. set channel ^ (1<<15)  (clear TUNE bit)

*/
func (s *Si4703) SetChannel(c float64) error {
    var err error
    var tmp, newc uint16

    if c < 87.5 || c > 107.9 {
        return ErrInvalidFreq
    }

    // 0 == 87.5 ... 5 == 88.5 ... 101 == 107.7 ... 102 == 107.9
    newc = uint16( (c-87.5) / 0.2 )

    tmp = s.Reg[CHANNEL]
    tmp &= 0xFE00     // mask off old channel
    tmp |= newc       // new channel
    tmp |= (1<<15)    // set TUNE bit
    s.Set(CHANNEL, tmp)

    deadline := time.Now().Add(time.Second * 5)
    for {
        if s.Reg[STATUSRSSI] & (1<<14) != 0 {
            break
        }
        if time.Now().After(deadline) {
            fmt.Println("can't tune", c, ": timed out!")
            return ErrTimeout
        }
        if ! s.Polling {
            if err = s.Read(); err != nil {
                log.Println(err)
                os.Exit(-1)
            }
        }
        time.Sleep(100 * time.Millisecond)
    }

    tmp = s.Reg[CHANNEL]
    tmp &= ^uint16(1<<15)  // clear TUNE bit
    s.Set(CHANNEL, tmp)
    return nil
}

func (s *Si4703) SetOsc(on bool) {
    if on {
        s.Set(OSCILLATOR, 0x8100)  // XOSCEN | ???
    } else {
        s.Set(OSCILLATOR, 0x0000)  // or should this be 0x0100?
    }
}

func (s *Si4703) Mute(on bool) {
    var tmp uint16

    // 0x4000 is the _disable mute_ flag, a zero means _mute enabled_
    if on  && (s.Reg[POWERCFG] & 0x4000 != 0x4000) ||
       !on && (s.Reg[POWERCFG] & 0x4000 == 0x4000) {
        // if mute already on/off
        return
    }
    if on {
        tmp = s.Reg[POWERCFG] & ^uint16(0x4000)
    } else {
        tmp = s.Reg[POWERCFG] | 0x4000
    }
    s.Set(POWERCFG, tmp)
}

func (s *Si4703) Enable() {
    if (s.Reg[POWERCFG] & 0x0001 == 0x0001) {
        return
    }
    // make sure the ENABLE bit is set, and the DISABLE bit is cleared
    // sleeping for 1.5ms just in case we are coming off of a shutdown
    time.Sleep(1500 * time.Microsecond)
    s.Set(POWERCFG, (s.Reg[POWERCFG] | 0x0001) & ^uint16(0x0040))
}

func (s *Si4703) Disable() {
    s.Set(POWERCFG, s.Reg[POWERCFG] | 0x0040)
}

func (s *Si4703) Volume(v int) {
    // the volext bit 0x0100 _reduces_ the maximum volume
    ext := s.Reg[SYSCONFIG3] & 0x0100 == 0x0100
    if v < 0 {
        v = 0
    } else if v > 31 {
        v = 31
    }
    newext := ! (v & 0x10 == 0x10)  // volext == 1 means LOWER volume
    newvol := uint16(v & 0x0F)
    if ext && ! newext {
        // volext quiet -> loud: set volume, then clear volext
        s.Set(SYSCONFIG2, (s.Reg[SYSCONFIG2] & 0xFFF0) | newvol)
        s.Set(SYSCONFIG3, s.Reg[SYSCONFIG3] & ^uint16(0x0100))
    } else if !ext && newext {
        // volext louder -> quieter: set volext, then set volume
        s.Set(SYSCONFIG3, s.Reg[SYSCONFIG3] | uint16(0x0100))
        s.Set(SYSCONFIG2, (s.Reg[SYSCONFIG2] & 0xFFF0) | newvol)
    } else {
        // volext not changing, just update the volume
        s.Set(SYSCONFIG2, (s.Reg[SYSCONFIG2] & 0xFFF0) | newvol)
    }
}

// 0a : STC tuning is complete, SF/BL indicates seek band rollover, ST indicates stereo
//     RDSR indicates RDS data ready, RSS[7:0] indicate RSSI for the current channel
//     15 is RDSR, 13 is ?valuesfbl?
// 0b : READCHAN[9:0] is the current channel,  15 14 13 12 11 10?

// a:STATUSRSSI b:READCHAN c:RDSA d:RDSB e:RDSC f:RDSD
// verbose mode?  a bit in POWERCFG (register 2)
// A : RDSR indicates RDS is ready, BLERA indicate how many errors were corrected
// B : BLERB BLERC BLERD bits indicate how many errors were corrected, if BLERB indicates more than 6 errors all 3 blocks should be discarded
// C-F : contain error-corrected data

/*
func Scan(s *Si4703) {
    var rssi, stereo float64
    var rdsr, traffic rune
    var call, prog string

    updates := 20
    for f:=87.5; f<107.9; f+=.2 {
        s.SetChannel(f)

        r := RDS{}
        call = ""
        prog = ""
        rdsr = ' '
        traffic = ' '
        rssi = 0
        stereo = 0
        for i:=0; i<updates; i++ {
            <-s.Update
            if s.Reg[STATUSRSSI] & 0x8000 == 0x8000 {
                rdsr = 'X'
                r.Update(s.Reg[RDSA], s.Reg[RDSB], s.Reg[RDSC], s.Reg[RDSD])
            }
            if s.Reg[STATUSRSSI] & 0x0010 == 0x0010 {
                stereo += 1
            }
            if r.TrafficProgram {
                traffic = 'T'
            }
            rssi += float64(s.Reg[STATUSRSSI] & 0xff)
        }
        if r.CallSign[0] != 0 {
            call = string(r.CallSign[:])
            prog = PT_NA[r.ProgramType]
        }
        fmt.Printf("%5.1f :  %4.1f %.2f %c %c  %4.4s  %-21.21s ",
            f, rssi/float64(updates), stereo/float64(updates), rdsr, traffic, call, prog) //r.CallSign[:], PT_NA[r.ProgramType])
        for i:=0; i<int(rssi/float64(updates)); i++ {
            fmt.Printf("x")
        }
        fmt.Println()
    }
}
*/

package main

/*

* rdsa: 16 bit PI Code; NA: encoded call sign, EU: country/coverage/program reference
* rdsb:
    * Group Type      : xxxx_...._...._....
    * Version         : ...._x..._...._....
    * Traffic Program : ...._.x.._...._....
    * Program Type    : ...._..xx_xxx._....
    * GT-dependent    : ...._...._...x_xxxx
* rdsc: GT-dependent (all Version B groups repeat PI here?)
* rdsd: GT-dependent

*/
/*
A "group" is 4 blocks of 26 bits (16bit information word, 10bit checksum and offset word), **64 bits of content per group**

RDS data always sent: Program Information (PI), Program Type (PTY), Traffic Program (TP)
RDS group types sometimes:
    Program Service (PS) name (0A, 0B)
    Alternative Frequency (AF) code pairs (0A)
    Traffic Announcement (TA) code (0A, 0B, 14B, 15B)
    Decoder identification (DI) code (0A, 0B, 15B)
    Music/speech (M/S) code (0A, 0B, 15B)
    Radiotext (RT) message (2A, 2B)
    Enhanced other networks information (EON) (14A)

(the cast majority of observed RDS group types is 0A/0B and 2A/2B)

> The PI field in block A and the PTY and TP fields within block B can be processed with every valid group. The group type, which can be used to selectively process specific groups, such as 0A and 2A shown in this example, is also available in block B.

* PI code: 4 hex digits code that is unique to each station, derived from call leters so no two are alike.
    * There are also regional/network variants of these codes
    * For example, it is possible for multiple stations to switch to the same PI for regional/national simulcast, then switch back afterwards
    * NPR has the national PI codes Bx01
* PS name: the name of the station, call letters, or a slogan.  Only 8 characters.
* TP: identifies that the station provides traffic reporting
* TA: when set to 1 when an actual traffic bulletin is broadcast
* PTY (program type): current program material being broadcast, codes 0..31
    * Dynamic PTY bit: indicates that the PTY can change
* PTYN (program type name): 8 character program name, progammed by station
* AF: alternate frequencies
    * PI code is stored with PS name and alternate frequencies
* RT: radio text, 64 chars
* Clock and date: 
* EWS (emergency warning system): coded emergency information
* ODC: open data channel, governed by AID code
* TDC: transparent data channel, specialized applications such as advertising sent to an electronic billboard
* In-House application: only for use by the broadcaster, ie telemetry, paging applications
* Radio Paging: numeric or alphanumeric
* TMC: traffic message channel, traffic information codes so that audio bulletins aren't required
* EON: data carried in groups 14A and 14B to indicate a cooperating station that provides traffic announcements, triggered with TA == 1; typically (always?) TP == 0, ie. no traffic program of it's own, but can still indicate an announcement on a sister station
    * 14A: provides PI,PS,TP,TA,PTY,PIN data for other stations
    * 14B: transmits a burst of 14B groups at the beginning of traffic announcements
*/

type RDS struct {
    // always
    ProgramInformation  uint16  // PI - encodes station ID
    CallSign            [4]byte // derived from PI
    ProgramType         int     // PTY - 0..31 code for station program type
    TrafficProgram      bool    // TP - station will broadcast traffic info
    TrafficAnnouncement bool    // TA - currently broadcasting traffic info
    Music               bool    // M/S - true if music
    Stereo              bool
    ArtificialHead      bool
    Compressed          bool
    DynamicPTY          bool

    // variable
    ProgramService      string  // PS - 64 chars, song title, artist, etc.
    AltFreqs            map[float64]bool  // AF
    NumAltFreqs         int
    Radiotext           string  // RT

    // application identification codes for open data applications
    aid    map[uint16][32]uint16

    // call sign triple buffering
    cs1    [4]byte
    cs2    [4]byte

    // program service triple buffering
    ps1    [8]byte
    ps2    [8]byte
    psnew  [8]bool

    // radiotext triple buffering
    rt1    [64]byte
    rt2    [64]byte
    rtnew  [64]bool
}

func (r *RDS) Update(rdsa, rdsb, rdsc, rdsd uint16) error {
    var group_type int
    var version byte

    r.update_pi(rdsa)

    group_type = int(rdsb>>12)
    if rdsb & 0x0800 != 0x0800 {
        version = 'A'
    } else {
        version = 'B'
    }
    r.TrafficProgram = rdsb & 0x20 == 0x20
    r.ProgramType = int((rdsb>>5) & 0x1f)

    switch group_type {
        case 0:
            // 0A, 0B : "Basic Tuning and Switching Information only"
            r.update_ps(rdsa, rdsb, rdsc, rdsd)
        case 1:
            // 1A, 1B : "Program Item Number and slow labeling codes"
            r.update_pin(rdsa, rdsb, rdsc, rdsd)
        case 2:
            // 2A, 2B : "Radio Text only"
            r.update_rt(rdsa, rdsb, rdsc, rdsd)
        case 3:
            if version == 'A' {
                // 3A : "Applications Identification for ODA only"
                r.update_aid(rdsa, rdsb, rdsc, rdsd)
            } else {
                // 3B : "Open Data Applications"
                // TODO
            }
        case 8:
            // Need the specs. From U.S. RBDS Standard - April 1998, pg. 32:
            //     The specification for TMC, using the so called ALERT Cprotocol also makes
            //     use of type 1A and/or type 3A groups together with 4A groups and is separately
            //     specified by theCEN standard ENV 12313-1.
            // Also, see pg 19.
        default:
//            fmt.Printf("%.4x %.4x %.4x %.4x   %.2d%c\n", rdsa, rdsb, rdsc, rdsd, group_type, version)
    }
    return nil
}
func (r *RDS) update_pi(rdsa uint16) {
    var tmp uint16
    var upd bool
    var i int

    // See: U.S. RBDS Standard - April 1998 ("rbds1998.pdf"), pg 80-90
    r.ProgramInformation = rdsa
    switch {
        case (rdsa & 0x0F00) == 0x0000:
            // _0__ : European local (unique) broadcast
            r.cs1[0] = 'A'
            r.cs1[1] = 65 + byte((rdsa>>12)&0xf)
            r.cs1[2] = 65 + byte((rdsa>>4)&0xf)
            r.cs1[3] = 65 + byte(rdsa&0xf)
        case (rdsa & 0x00FF) == 0x0000:
            // __00 : European test modes
            r.cs1[0] = 'A'
            r.cs1[1] = 'F'
            r.cs1[2] = 65 + byte((rdsa>>12)&0xf)
            r.cs1[3] = 65 + byte((rdsa>>8)&0xf)
//        case  NA  nationally linked stations: first nybble is b/d/f, seccond nybble is 1..f, last byte is 01..ff
//        case  NA 3-char stations
        case (rdsa >= 4096) && (rdsa <= 39247):
            // North American 4-digit "W" and "K" stations
            if rdsa < 21672 {
                r.cs1[0] = 'K'
                tmp = rdsa-4096
            } else {
                r.cs1[0] = 'W'
                tmp = rdsa-21672
            }
            r.cs1[1] = 65 + byte(tmp/676)
            tmp %= 676
            r.cs1[2] = 65 + byte(tmp/26)
            tmp %= 26
            r.cs1[3] = 65 + byte(tmp)
    }

    // triple buffer, only update if we've seen the same thing twice
    upd = true
    for i=0; i<4; i++ {
       if r.cs1[i] != r.cs2[i] {
           upd = false
       }
    }
    if upd {
        for i=0; i<4; i++ {
            r.CallSign[i] = r.cs2[i]
        }
    }
    for i=0; i<4; i++ {
        r.cs2[i] = r.cs1[i]
    }
}

/*
Register application identification for a ODA group
*/
func (r *RDS) update_aid(rdsa, rdsb, rdsc, rdsd uint16) {
    if r.aid == nil {
        r.aid = map[uint16][32]uint16{}
    }
    if _, ok := r.aid[rdsa]; !ok {
        r.aid[rdsa] = [32]uint16{}
    }

    oda_group := int(rdsb & 0x1f)
// BROKEN
    aidmap := r.aid[rdsa]
    aidmap[oda_group] = rdsd
}

func (r *RDS) update_ps(rdsa, rdsb, rdsc, rdsd uint16) {
    var i, idx int
    var upd bool

    //// Music and TA flags
    r.Music = (rdsb & 0x0008) == 0x0008
    r.TrafficAnnouncement = (rdsb & 0x0010) == 0x0010

    //// Program Service
    for i, _ = range r.psnew {
        r.psnew[i] = false
    }
    idx = int(rdsb & 0x3) * 2
    r.ps1[idx] = byte((rdsd>>8) & 0x7f)
    r.ps1[idx+1] = byte(rdsd & 0x7f)
    r.psnew[idx] = true
    r.psnew[idx+1] = true

    if idx == 0 {
        // triple buffer, only update if we've seen the same thing twice
        upd = true
        for i=0; i<8; i++ {
            if r.ps1[i] != r.ps2[i] {
                upd = false
            }
        }
        if upd {
            r.ProgramService = string(r.ps2[0:8])
        }
        for i=0; i<8; i++ {
            r.ps2[i] = r.ps1[i]
        }
    }

    //// Decoder Information
    switch rdsb & 0x3 {
        case 0: r.Stereo = (rdsb & 0x4) == 0x4
        case 1: r.ArtificialHead = (rdsb & 0x4) == 0x4 
        case 2: r.Compressed = (rdsb & 0x4) == 0x4
        case 3: r.DynamicPTY = (rdsb & 0x4) == 0x4
    }

    //// Alternative Frequencies
    if rdsb & 0x0800 != 0x0800 {
        // 0A 
        if r.AltFreqs == nil {
            r.AltFreqs = map[float64]bool{}
        }
        for _, f := range []uint16{rdsc>>8, rdsc&0xff} {
            switch {
                case f==0 || (f>=205 && f<=223) || (f>=251 && f<=255):
                    // PASS: "not to be used" (0), "filler code" (205), "not assigned"
                case f>=1 && f<=204:
                    r.AltFreqs[87.5 +(float64(f)*.1)] = true
                case f>=224 && f<=249:
                    r.NumAltFreqs = int(f-224)
                case f==205:
                    // LF/MF frequency follows

            }
        }
    }
    // else 0B: rdsc == rdsa
}
func (r *RDS) update_pin(rdsa, rdsb, rdsc, rdsd uint16) {
// TODO: stopping here...
/*
   var rpc, slc, d, h, m int
   rpc = int(rdsb & 0x1f)
   slc = int(rdsc)
   d = int((rdsd>>11) & 0x1f)
   h = int((rdsd>>6) & 0x1f)
   m = int(rdsd & 0x3f)
*/
//   fmt.Printf("PIN: %d %.4x %d %d %d\n", rpc, slc, d, h, m)
}

func (r *RDS) update_rt(rdsa, rdsb, rdsc, rdsd uint16) {
    var idx, cridx, i  int
    var msgbytes [4]byte
    var upd bool

    idx = int(rdsb & 0xf) * 4
    msgbytes[0] = byte((rdsc>>8) & 0x7f)
    msgbytes[1] = byte(rdsc & 0x7f)
    msgbytes[2] = byte((rdsd>>8) & 0x7f)
    msgbytes[3] = byte(rdsd & 0x7f)

    for i, _ = range r.rtnew {
        r.rtnew[i] = false
    }

    cridx = -1
    for i=0; i<4; i++ {
        r.rt1[idx+i] = msgbytes[i]
        r.rtnew[idx+i] = true
        // 0x0d == CR (carriage return)
        if msgbytes[i] == 0x0d {
            cridx = idx+i
        }
    }

    // received a CR, clear everything afterwards
    if cridx != -1 {
        for i=cridx+1; i<len(r.rt1); i++ {
            r.rt1[i] = ' '
        }
    }

    if idx == 0 {
        // triple buffer, only update if we've seen the same thing twice
        upd = true
        for i=0; i<64; i++ {
            if r.rt1[i] != r.rt2[i] {
                upd = false
            }
        }
        if upd {
            for i=0; i<len(r.rt2) && r.rt2[i] != 0x0d; i++ {
                // i stops at the first CR or the end
            }
            r.Radiotext = string(r.rt2[0:i])
        }
        for i=0; i<64; i++ {
            r.rt2[i] = r.rt1[i]
        }
    }
}

var PT_NA [32]string = [32]string {
    "No program type",
    "News",
    "Information",
    "Sports",
    "Talk",
    "Rock",
    "Classic Rock",
    "Adult Hits",
    "Soft Rock",
    "Top 40",
    "Country",
    "Oldies",
    "Soft",
    "Nostalgia",
    "Jazz",
    "Classical",
    "Rhythm and Blues",
    "Soft Rhythm and Blues",
    "Language",
    "Religious Music",
    "Religious Talk",
    "Personality",
    "Public",
    "College",
    "Unassigned 24",
    "Unassigned 25",
    "Unassigned 26",
    "Unassigned 27",
    "Unassigned 28",
    "Weather",
    "Emergency Test",
    "Emergency",
}

var PT_EU [32]string = [32]string {
    "No program type",
    "News",
    "Current Affairs",
    "Information",
    "Sport",
    "Education",
    "Drama",
    "Culture",
    "Science",
    "Varied",
    "Pop Music",
    "Rock Music",
    "M.O.R. Music",
    "Light Classical",
    "Serious Classical",
    "Other Music",
    "Weather",
    "Finance",
    "Children's Programs",
    "Social Affairs",
    "Religion",
    "Phone-In",
    "Travel",
    "Leisure",
    "Jazz Music",
    "Country Music",
    "National Music",
    "Oldies Music",
    "Folk Music",
    "Documentary",
    "Alarm test",
    "Alarm",
}

var GroupTypesA [16]string = [16]string{
    "Basic Tuning and Switching Information only",
    "Program Item Number and Slow Labeling Codes only",
    "Radio Text only",
    "Applications Identification for ODA only",
    "Clock Time and Date only",
    "Transparent Data Channels (32 channels) or ODA",
    "In-House Applications of ODA",
    "Radio Paging of ODA",
    "Traffic Message Channel or ODA",
    "Emergency Warning System or ODA",
    "Program Type Name",
    "Open Data Applications",
    "Open Data Applications",
    "Enhanced Radio Paging or ODA",
    "Enhanced Other Networks Information Only",
    "Defined in RBDS only",
}

var GroupTypesB [16]string = [16]string{
    "Basic Tuning and Switching Information only",
    "Program Item Number",
    "Radio Text only",
    "Open Data Applications",
    "Open Data Applications",
    "Transparent Data Channels (32 channels) or ODA",
    "In-House Applications of ODA",
    "Radio Paging of ODA",
    "Open Data Applications",
    "Open Data Applications",
    "Open Data Applications",
    "Open Data Applications",
    "Open Data Applications",
    "Open Data Applications",
    "Enhanced Other Networks Information Only",
    "Fast Switching Information only",
}


package hls

import (
	"bytes"
	"container/ring"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	. "github.com/Monibuca/engine/v3"
	"github.com/Monibuca/utils/v3"
	"github.com/Monibuca/utils/v3/codec"
	"github.com/Monibuca/utils/v3/codec/mpegts"
)

var memoryTs sync.Map
var memoryM3u8 sync.Map
var PlaylistSequence sync.Map

func writeHLS(r *Stream) {
	if filterReg != nil && !filterReg.MatchString(r.StreamPath) {
		return
	}
	var m3u8Buffer bytes.Buffer
	var infoRing = ring.New(config.Window)

	memoryM3u8.Store(r.StreamPath, &m3u8Buffer)
	defer memoryM3u8.Delete(r.StreamPath)
	var err error
	var hls_fragment int64       // hls fragment
	var hls_segment_count uint32 // hls segment count
	var vwrite_time uint32
	var video_cc, audio_cc uint16
	var outStream = Subscriber{ID: "HLSWriter", Type: "HLS"}

	if err = outStream.Subscribe(r.StreamPath); err != nil {
		utils.Println(err)
		return
	}
	vt := outStream.WaitVideoTrack("h264")
	at := outStream.WaitAudioTrack("aac")
	if err != nil {
		return
	}
	var asc codec.AudioSpecificConfig
	if at != nil {
		asc, err = decodeAudioSpecificConfig(at.ExtraData)
	}
	if err != nil {
		return
	}
	if config.Fragment > 0 {
		hls_fragment = config.Fragment * 1000
	} else {
		hls_fragment = 10000
	}

	hls_playlist := Playlist{
		Writer:         &m3u8Buffer,
		Version:        3,
		Sequence:       0,
		Targetduration: int(hls_fragment / 666), // hlsFragment * 1.5 / 1000
	}
	hls_path := filepath.Join(config.Path, r.StreamPath, fmt.Sprintf("%d.m3u8", time.Now().Unix()))
	os.MkdirAll(filepath.Dir(hls_path), 0755)
	var file *os.File
	file, err = os.OpenFile(hls_path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	record_playlist := Playlist{
		Writer:         file,
		Version:        3,
		Sequence:       0,
		Targetduration: int(hls_fragment / 666), // hlsFragment * 1.5 / 1000
	}
	if err = hls_playlist.Init(); err != nil {
		return
	}
	if err = record_playlist.Init(); err != nil {
		return
	}
	hls_segment_data := &bytes.Buffer{}
	outStream.OnVideo = func(ts uint32, pack *VideoPack) {
		packet, err := VideoPacketToPES(ts, pack, vt.ExtraData.NALUs)
		if err != nil {
			return
		}
		if pack.IDR {
			// 当前的时间戳减去上一个ts切片的时间戳
			if int64(ts-vwrite_time) >= hls_fragment {

				// 计算 TS 分片，默认从0开始，会一直保存在内存中，直到收到删除流的请求
				var seq = int(0)
				v, ok := PlaylistSequence.Load(r.StreamPath)
				if ok {
					log.Printf("playlist sequence %s:%#v\n", r.StreamPath, v)
					seq = v.(int) + 1
				}
				PlaylistSequence.Store(r.StreamPath, seq)

				tsFilename := fmt.Sprintf("%d.ts", seq)
				tsData := hls_segment_data.Bytes()
				tsFilePath := filepath.Join(filepath.Dir(hls_path), tsFilename)
				if config.EnableWrite {
					if err = writeHlsTsSegmentFile(tsFilePath, tsData); err != nil {
						return
					}
				}
				if config.EnableMemory {
					memoryTs.Store(tsFilePath, tsData)
				}
				inf := PlaylistInf{
					//浮点计算精度
					Duration: float64((ts - vwrite_time) / 1000.0),
					Title:    filepath.Base(filepath.Dir(hls_path)) + "/" + tsFilename,
					FilePath: tsFilePath,
				}

				if hls_segment_count >= uint32(config.Window) {
					m3u8Buffer.Reset()
					if err = hls_playlist.Init(); err != nil {
						return
					}
					memoryTs.Delete(infoRing.Value.(PlaylistInf).FilePath)
					infoRing.Value = inf
					infoRing = infoRing.Next()
					infoRing.Do(func(i interface{}) {
						hls_playlist.WriteInf(i.(PlaylistInf))
					})
				} else {
					infoRing.Value = inf
					infoRing = infoRing.Next()
					if err = hls_playlist.WriteInf(inf); err != nil {
						return
					}
				}
				inf.Title = tsFilename
				if err = record_playlist.WriteInf(inf); err != nil {
					return
				}
				hls_segment_count++
				vwrite_time = ts
				hls_segment_data.Reset()
			}
		}

		frame := new(mpegts.MpegtsPESFrame)
		frame.Pid = 0x101
		frame.IsKeyFrame = pack.IDR
		frame.ContinuityCounter = byte(video_cc % 16)
		frame.ProgramClockReferenceBase = uint64(ts) * 90
		if err = mpegts.WritePESPacket(hls_segment_data, frame, packet); err != nil {
			return
		}

		video_cc = uint16(frame.ContinuityCounter)
	}
	outStream.OnAudio = func(ts uint32, pack *AudioPack) {
		var packet mpegts.MpegTsPESPacket
		if packet, err = AudioPacketToPES(ts, pack.Raw, asc); err != nil {
			return
		}

		frame := new(mpegts.MpegtsPESFrame)
		frame.Pid = 0x102
		frame.IsKeyFrame = false
		frame.ContinuityCounter = byte(audio_cc % 16)
		//frame.ProgramClockReferenceBase = 0
		if err = mpegts.WritePESPacket(hls_segment_data, frame, packet); err != nil {
			return
		}
		audio_cc = uint16(frame.ContinuityCounter)
	}
	outStream.Play(at, vt)

	if config.EnableMemory {
		infoRing.Do(func(i interface{}) {
			if i != nil {
				memoryTs.Delete(i.(PlaylistInf).FilePath)
			}
		})
	}
}

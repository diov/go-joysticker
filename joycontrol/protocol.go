package joycontrol

import (
	"errors"
	"syscall"
	"time"

	"dio.wtf/joycontrol/joycontrol/log"
	"golang.org/x/sys/unix"
)

type Protocol struct {
	lastTime           time.Time
	elapsed            int64
	reportReceived     bool
	deviceInfoRequired bool
	imuEnabled         bool

	queue  chan *InputReport
	output *OutputReport

	itr, ctrl int
	macAddr   []byte
}

func NewProtocol() *Protocol {
	return &Protocol{
		queue:  make(chan *InputReport, 5),
		output: &OutputReport{},
	}
}

func (p *Protocol) Setup(itr, ctrl int, macAddr []byte) {
	p.itr = itr
	p.ctrl = ctrl
	p.macAddr = macAddr

	if err := unix.SetNonblock(p.itr, true); nil != err {
		log.Error(err)
		return
	}

	go p.sendEmptyReport()
	go p.processInputQueue()
	go p.readOutputReport()
}

func (p *Protocol) sendEmptyReport() {
	ticker := time.NewTicker(time.Second)

	<-ticker.C
	p.processStandardFullReport()
	ticker.Stop()
}

func (p *Protocol) processInputQueue() {
	for {
		report := <-p.queue
		if _, err := unix.Write(p.itr, (*report)[:]); nil != err {
			log.ErrorF("error writing input report: %v", err)
		} else {
			log.DebugF("input report written %s", report)
		}
	}
}

func (p *Protocol) readOutputReport() {
	// TODO: use EPOLL to improve performance
	for {
		err := p.output.load(p.itr)
		if err != nil {
			switch {
			case errors.Is(err, syscall.EAGAIN):
				continue
			case errors.Is(err, errEmptyData), errors.Is(err, errBadLengthData), errors.Is(err, errMalformedData):
				// TODO: Setting Report ID to full standard input report ID
				p.processStandardFullReport()
				return
			default:
				log.ErrorF("error reading output report: %v", err)
				return
			}
		}

		p.reportReceived = true
		log.DebugF("output report read %s", p.output)
		switch p.output.id {
		case RumbleAndSubcommand:
			p.processSubcommandReport(p.output)
		case UpdateNFCPacket:
		case RumbleOnly:
		case RequestNFCData:
		}
	}
}

func (p Protocol) processStandardFullReport() {
	report := AllocStandardReport()
	report.setReportId(StandardFullMode)
	report.setImuData(p.imuEnabled)
	p.queue <- report
}

func (p *Protocol) processSubcommandReport(report *OutputReport) {
	p.updateTimer()

	subcommand := report.getSubcommand()
	switch subcommand {
	case RequestDeviceInfo:
		p.answerDeviceInfo()
	case SetInputReportMode:
		p.answerSetMode(report.getSubcommandData())
	case TriggerButtonsElapsedTime:
		p.anwserTriggerButtonsElapsedTime()
	case SetShipmentLowPowerState:
		p.answerSetShipmentState()
	case SpiFlashRead:
		p.answerSpiRead(report.getSubcommandData())
	case SetNfcMcuConfig:
		p.answerSetNfcMcuConfig(report.getSubcommandData())
	case SetNfcMcuState:
		p.answerSetNfcMcuState(report.getSubcommandData())
	case SetPlayerLights:
		p.answerSetPlayerLights()
	case EnableImu:
		p.answerEnableImu(report.getSubcommandData())
	case EnableVibration:
		p.answerEnableVibration()
	default:
		// Currently set so that the controller ignores any unknown
		// subcommands. This is better than sending a NACK response
		// since we'd just get stuck in an infinite loop arguing
		// with the Switch.
		p.processStandardFullReport()
	}
}

func (p *Protocol) answerSetMode(data []byte) {
	// TODO: Update input report mode
	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackSetInputReportMode()
	p.queue <- report
}

func (p *Protocol) anwserTriggerButtonsElapsedTime() {
	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackTriggerButtonsElapsedTime()
	p.queue <- report
}

func (p *Protocol) answerDeviceInfo() {
	p.deviceInfoRequired = true

	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackDeviceInfo(p.macAddr)
	p.queue <- report
}

func (p *Protocol) answerSetShipmentState() {
	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackSetShipmentLowPowerState()
	p.queue <- report
}

func (p *Protocol) answerSpiRead(data []byte) {
	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackSpiFlashRead(data)
	p.queue <- report
}

func (p *Protocol) answerSetNfcMcuConfig(data []byte) {
	// TODO: Update NFC MCU config
	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackSetNfcMcuConfig()
	p.queue <- report
}

func (p *Protocol) answerSetNfcMcuState(data []byte) {
	// TODO: Update NFC MCU State
	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackSetNfcMcuState()
	p.queue <- report
}

func (p *Protocol) answerSetPlayerLights() {
	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackSetPlayerLights()
	p.queue <- report
}

func (p *Protocol) answerEnableImu(data []byte) {
	if data[0] == 0x01 {
		p.imuEnabled = true
	}

	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackEnableImu()
	p.queue <- report
}

func (p *Protocol) answerEnableVibration() {
	report := AllocStandardReport()
	report.setReportId(SubcommandReplies)
	report.fillStandardData(p.elapsed, p.deviceInfoRequired)
	report.ackEnableVibration()
	p.queue <- report
}

func (p *Protocol) updateTimer() {
	now := time.Now()
	duration := now.Sub(p.lastTime)

	p.elapsed = int64(duration/4) & 0xFF
	p.elapsed = 0xFF
	p.lastTime = now
}

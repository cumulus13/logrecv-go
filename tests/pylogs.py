#!/usr/bin/env python3

# File: tests/pylogs.py
# Author: Hadi Cahyadi <cumulus13@gmail.com>
# Date: 2026-10-28
# Description: 
# License: MIT

import logging
import logging.handlers

# 1. Define unique numeric values for custom levels without breaking standard Python levels
# Python default: DEBUG=10, INFO=20, WARNING=30, ERROR=40, CRITICAL=50
NOTICE_LEVEL = 25
ALERT_LEVEL = 55
EMERGENCY_LEVEL = 60

logging.addLevelName(NOTICE_LEVEL, "NOTICE")
logging.addLevelName(ALERT_LEVEL, "ALERT")
logging.addLevelName(EMERGENCY_LEVEL, "EMERGENCY")

# 2. Handler Configuration
h = logging.handlers.SysLogHandler(address=("127.0.0.1", 5514))

formatter = logging.Formatter('TEST-LOGRECV: %(message)s')
h.setFormatter(formatter)

# 3. Map each level name to the correct syslog priority key
h.mapPriority = lambda levelName: {
    'DEBUG': 'debug',
    'INFO': 'info',
    'NOTICE': 'notice',
    'WARNING': 'warning',
    'ERROR': 'err',
    'CRITICAL': 'crit',    # CRITICAL maps to Syslog 'crit' (Critical)
    'ALERT': 'alert',      # ALERT maps to Syslog 'alert'
    'EMERGENCY': 'emerg',  # EMERGENCY maps to Syslog 'emerg' (Emergency)
}.get(levelName, 'warning')

logger = logging.getLogger("TEST")
logger.setLevel(logging.DEBUG)
logger.addHandler(h)

# 4. Helper functions for custom levels
def notice(self, message, *args, **kwargs):
    if self.isEnabledFor(NOTICE_LEVEL):
        self._log(NOTICE_LEVEL, message, args, **kwargs)

def alert(self, message, *args, **kwargs):
    if self.isEnabledFor(ALERT_LEVEL):
        self._log(ALERT_LEVEL, message, args, **kwargs)

def emergency(self, message, *args, **kwargs):
    if self.isEnabledFor(EMERGENCY_LEVEL):
        self._log(EMERGENCY_LEVEL, message, args, **kwargs)

logging.Logger.notice = notice
logging.Logger.alert = alert
logging.Logger.emergency = emergency

# Test logs:
logger.emergency("This is an EMERGENCY message")
logger.alert("This is an ALERT message")
logger.critical("This is a CRITICAL message")
logger.error("This is an ERROR message")
logger.warning("This is a WARNING message")
logger.notice("This is a NOTICE message")
logger.info("This is a system INFO message!")
logger.debug("This is a DEBUG message!")
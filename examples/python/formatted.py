import logging
import socket
from logging.handlers import SysLogHandler

HOST = socket.gethostname()

handler = SysLogHandler(address=("127.0.0.1", 5514), facility=SysLogHandler.LOG_USER)
# RFC 3164 style header so logrecv can show host, app name and pid in their own colors
handler.setFormatter(logging.Formatter(
    f"%(asctime)s {HOST} %(name)s[%(process)d]: %(message)s",
    datefmt="%b %d %H:%M:%S",
))

log = logging.getLogger("myapp")
log.setLevel(logging.DEBUG)
log.addHandler(handler)

log.info("user logged in")
log.error("payment failed")
try:
    1 / 0
except ZeroDivisionError:
    log.exception("boom")  # the traceback is shown with the same level/host/app

import logging
import socket
from logging.handlers import SysLogHandler

# TCP instead of UDP: reliable delivery, same message format
handler = SysLogHandler(address=("127.0.0.1", 5514), socktype=socket.SOCK_STREAM)

log = logging.getLogger("myapp")
log.setLevel(logging.DEBUG)
log.addHandler(handler)

log.info("one")
log.error("two")

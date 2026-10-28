import logging
from logging.handlers import SysLogHandler

log = logging.getLogger("myapp")
log.setLevel(logging.DEBUG)
log.addHandler(SysLogHandler(address=("127.0.0.1", 5514)))

log.debug("cache miss")
log.info("user logged in")
log.warning("disk almost full")
log.error("payment failed")
log.critical("queue stalled")

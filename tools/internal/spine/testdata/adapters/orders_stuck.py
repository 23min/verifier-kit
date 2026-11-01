"""orders.py stuck on the way: it refuses ship, so shipped and delivered are never reached."""
import orders
from orders import new, apply, observe

orders.BROKEN = "ship"

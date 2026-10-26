# Each slip in #64's table, citing a cell whose hash is current; each is bad-citation, not read.
# covers closed/open@26ccaba33ea0
# covers: closed/open @26ccaba33ea0
# cover: closed/open@26ccaba33ea0
# Covers: closed/open@26ccaba33ea0
# Text that only mentions covering, reported as nothing and read as nothing:
# This test covers: edge cases, and the checks cover the text.
# Longer words are not the word: coverage@7.4, COVERSHEET@x.
# A comment that ends with what it covers:
@decorator
# or that ends in covers
@decorator
# A citation split over two lines is neither read nor reported, so closed/remove stays uncovered: covers:
closed/remove@c4ec20806a30
# It discovers: closed/remove@c4ec20806a30 is not the word covers, so closed/remove stays uncovered.
# The one citation here: covers: closed/open@26ccaba33ea0

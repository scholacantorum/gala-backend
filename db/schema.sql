-- The gtable table has a row for each table, or potential table, at the event.
-- Every party is seated at some potential table.
CREATE TABLE gtable (
    -- Unique identifier of the table.  Note this is not the visible table
    -- number.
    id integer PRIMARY KEY,

    -- Position of the table on the tables page (pixels, with origin at top
    -- left).  (0, 0) means no position assigned.
    x integer NOT NULL DEFAULT 0,
    y integer NOT NULL DEFAULT 0,

    -- Table number (visible).  Zero means not assigned.
    num integer NOT NULL DEFAULT 0,

    -- Table name.
    name text NOT NULL DEFAULT ''
);

-- The party table has a row for each party of guests that should be seated
-- together.  Every guest is a member of a party, even if it's a party of one.
CREATE TABLE party (
    -- Unique identifier of the party.
    id integer PRIMARY KEY,

    -- Table at which the party is seated.
    gtable integer NOT NULL REFERENCES gtable,

    -- Placement at that table, to ensure consistent layout.
    place integer NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX party_place_idx ON party (gtable, place);

-- The guest table has a row for each guest at the gala.
CREATE TABLE guest (
    -- Unique identifier of a guest (not visible to users).
    id integer PRIMARY KEY,

    -- Name of the guest (natural order).  If the guest's name is not known at
    -- registration time, they'll be given a name like "Steve Roth Guest #1".
    name text NOT NULL
        CHECK (name != ''),

    -- Name of the guest, last name first for sorting.
    sortname text NOT NULL
        CHECK (sortname != ''),

    -- Email address of the guest.  May be empty.
    email text NOT NULL DEFAULT '',

    -- Postal address of the guest.  Must specify all four columns, or none.
    address text NOT NULL DEFAULT '',
    city    text NOT NULL DEFAULT '' CHECK((address='') = (city='')),
    state   text NOT NULL DEFAULT '' CHECK((address='') = (state='')),
    zip     text NOT NULL DEFAULT '' CHECK((address='') = (zip='')),

    -- Phone number of the guest.  May be empty.
    phone text NOT NULL DEFAULT '',

    -- Special requests entered by the guest.  May have embedded newlines.
    requests text NOT NULL DEFAULT '',

    -- Party (seating group) to which the guest belongs.
    party integer NOT NULL REFERENCES party,

    -- Bidder number for the guest, or 0 if not yet assigned.  Should be unique
    -- except in cases where one guest delegates payment to another; in that
    -- case the two *may* have the same bidder number.  Note that this is
    -- presented to the user in hexadecimal, because sometimes there are tables
    -- with more than 10 bidders.  Bidder numbers for table 12 (decimal) will
    -- range from 0x120 to 0x12F.
    bidder integer NOT NULL DEFAULT 0,

    -- Stripe customer ID, if this guest is a customer in Stripe (otherwise
    -- empty).
    stripeCustomer text NOT NULL DEFAULT '',

    -- Stripe source ID for the default card for the Stripe customer.  Non-empty
    -- if and only if stripeCustomer is non-empty.  Note that this is cached and
    -- could be stale, if the customer changes their card while making a
    -- purchase on the Schola web site.
    stripeSource text NOT NULL DEFAULT ''
        CHECK ((stripeCustomer='') = (stripeSource='')),

    -- Description of the default card for the Stripe customer, e.g.  "Visa
    -- ending 4242".  Non-empty if and only if stripeCustomer is non-empty.
    -- Note that this is cached and could be stale, if the customer changes
    -- their card while making a purchase on the Schola web site.
    stripeDescription text NOT NULL DEFAULT ''
        CHECK ((stripeCustomer='') = (stripeDescription='')),

    -- Flag indicating that the customer has given approval to use their card
    -- for gala purchases.
    useCard boolean NOT NULL DEFAULT 0
        CHECK (stripeCustomer!='' OR NOT useCard),

    -- Guest ID of the guest who will pay for this guest's purchases.  NULL if
    -- the guest will pay for their own purchases (or hasn't specified payment
    -- yet).
    payer integer REFERENCES guest
        CHECK (payer IS NULL OR NOT useCard)
        CHECK (payer!=id),

    -- Entree is the guest's choice of entree.
    entree text NOT NULL DEFAULT ''
);
CREATE INDEX guest_bidder_idx ON guest (bidder);
CREATE INDEX guest_party_idx  ON guest (party);
CREATE INDEX guest_payer_idx  ON guest (payer);

-- The item table has a row for each thing that can be purchased or donated at
-- the gala: essentially each registration type, each auction item, and each
-- fund-a-need level.
CREATE TABLE item (
    -- Unique identifier of the item.
    id integer PRIMARY KEY,

    -- Name of the item (as it should appear on receipts and in the GUI).
    name text NOT NULL,

    -- Amount to be paid by the purchaser, in cents, if that is a fixed price
    -- (e.g. for an item representing a fund-a-need level).  If the amount is
    -- not a fixed price (e.g. a silent auction item whose amount will be the
    -- winning bid amount), this is zero.
    amount integer NOT NULL DEFAULT 0,

    -- Value of the goods and/or services included in the item, i.e., the amount
    -- that is *not* tax-deductible, in cents.  This will be zero for items that
    -- are purely donations (e.g. fund-a-need levels).
    value integer NOT NULL DEFAULT 0
);
INSERT INTO item (id, name, amount, value) VALUES
    (1, 'Registration', 17500, 5000);

-- The purchase table has a row for each purchase of an item.
CREATE TABLE purchase (
    -- Unique identifier of the purchase.
    id integer PRIMARY KEY,

    -- Identifier of the guest who purchased the item.  Note that, if multiple
    -- guests have the bidder number that purchased the item, this will identify
    -- the one of those guests for whom guest.payer=NULL, i.e., the one who's
    -- actually going to pay for it.
    guest integer NOT NULL REFERENCES guest,

    -- Guest who will pay for the purchase.  This may be different from the
    -- guest who made the purchase (e.g. bidder number 23 won the auction but
    -- bidder number 24, their spouse, is paying the bill).
    payer integer NOT NULL REFERENCES guest,

    -- Identifier of the item purchased.
    item integer NOT NULL REFERENCES item,

    -- Purchase amount for the item, in cents.
    amount integer NOT NULL
        CHECK (amount > 0),

    -- Date and time of the payment, in RFC3339 format.  Also serves as the flag
    -- for whether the purchase has been paid; an empty string means not paid.
    paymentTimestamp text NOT NULL DEFAULT '',

    -- Description of the payment method.  For credit card charges, this is
    -- "Visa 2345".  For other payment methods, this is a free-form string.  It
    -- is empty if the purchase hasn't been paid.
    paymentDescription text NOT NULL DEFAULT ''
        CHECK((paymentDescription='') = (paymentTimestamp='')),

    -- Schola order number, or zero if none.
    scholaOrder integer NOT NULL DEFAULT 0
);
CREATE INDEX purchase_guest_idx ON purchase (guest);
CREATE INDEX purchase_payer_idx ON purchase (payer);
CREATE INDEX purchase_item_idx  ON purchase (item);

-- The user table has one row for each person authorized to use the gala
-- management system.
CREATE TABLE user (
    -- Unique identifier of the user.
    id integer PRIMARY KEY,

    -- Username of the user.
    username text NOT NULL UNIQUE,

    -- Password for the user, in bcrypt format.
    password text NOT NULL
);
INSERT INTO user VALUES (1, 'sroth', '$2a$10$rfRymy4A0lsILBJN6U4r4.qhzsktWGAOl2NIACJJvyLQOO4uLmI0m');

-- The session table has one row for each valid session token.
CREATE TABLE session (
    -- Session token (a random string used as the session cookie value).
    token text PRIMARY KEY,

    -- Identifier of the user logged into this session.
    user integer NOT NULL REFERENCES user ON DELETE CASCADE,

    -- Time when the session expires (seconds since epoch).
    expires integer NOT NULL
);
CREATE INDEX session_user_idx    ON session (user);
CREATE INDEX session_expires_idx ON session (expires);

-- The journal table has one row for each transaction that changes the bidder,
-- group, guest, item, purchase, or payment tables.
CREATE TABLE journal (
    -- Unique identifier (and sequence number) of the journal entry.
    id integer PRIMARY KEY,

    -- Username of the user who made the change journaled in this entry.  This
    -- may be NULL if the change was a patron registering through the public
    -- site.
    user text REFERENCES user (username),

    -- Timestamp of the change, in RFC3339 format.
    timestamp text NOT NULL,

    -- Details of the change, as a JSON-encoded array.  Each element of the
    -- array is an object with "type" and "id" keys identifying the object type
    -- (group, guest, item, purchase, or payment) and the ID of an object that
    -- was changed in this transaction.  The remaining keys of the object are
    -- the modified properties of the object and their new values.
    -- Alternatively, the object may have a key "DELETE", with value true, which
    -- indicates that the object in question was deleted.
    change text NOT NULL -- JSON
);
CREATE INDEX journal_user_idx ON journal (user);

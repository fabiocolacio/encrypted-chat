package server

import(
    "fmt"
    "database/sql"
    "crypto/hmac"
    "crypto/sha256"
    "errors"
    "bytes"
    "log"
)

var(
    ErrNoSuchUser error = errors.New("No such user")
    ErrMsgUnsent error = errors.New("Failed to send message")
    ErrInvalidResponse error = errors.New("Invalid response to challenge")
)

func (serv *Server) InitDB() (err error) {
    query := fmt.Sprintf(
        `create table users(
            uid int primary key auto_increment,
            username varchar(%d),
            salt binary(%d),
            saltedhash binary(%d),
            challenge binary(%d));`,
        UsernameMaxLength,
        SaltLength,
        KeyHashLength,
        ChallengeLength)
    _, err = serv.db.Exec(query)
    if err != nil {
        return
    }

    _, err = serv.db.Exec(`create table messages(
        sender int,
        recipient int,
        timesent timestamp,
        message blob);`)
    if err != nil {
        return
    }

    return
}

func (serv *Server) ResetDB() (err error) {
    _, err = serv.db.Exec(`drop table if exists users;`)
    if err != nil {
        return
    }

    _, err = serv.db.Exec(`drop table if exists messages;`)
    if err != nil {
        return
    }

    err = serv.InitDB()

    return
}

func (serv *Server) ParseChallenge(user string, response []byte) error {
    var saltedHash []byte
    var challenge []byte

    row := serv.db.QueryRow("SELECT saltedhash, challenge FROM users WHERE username = ?", user)

    err := row.Scan(&saltedHash, &challenge)
    if err != nil {
        log.Println(user)
        return err
    }

    mac := hmac.New(sha256.New, saltedHash)
    mac.Write(challenge)
    expectedResponse := mac.Sum(nil)
    if Memcmp(expectedResponse, response) {
        return nil
    }

    return ErrInvalidResponse
}

func (serv *Server) SetChallenge(user string, challenge []byte) ([]byte, error) {
    _, err := serv.db.Exec(`UPDATE users SET challenge = ? WHERE username = ?`, challenge, user)
    if err != nil {
        return nil, err
    }
    var salt []byte
    row := serv.db.QueryRow(`SELECT salt FROM users WHERE username = ?`, user)
    err = row.Scan(&salt)
    return salt, err
}

func (serv *Server) MsgFetch(yourName string, myUid int, since string) ([]byte, error) {
    var(
        rows *sql.Rows
        data []byte
        myName string
        yourUid int
    )

    row := serv.db.QueryRow(`select uid from users where username = ?`, yourName)
    err := row.Scan(&yourUid)
    if err == sql.ErrNoRows {
        err = ErrNoSuchUser
        return data, err
    }

    row = serv.db.QueryRow(`select username from users where uid = ?`, myUid)
    err = row.Scan(&myName)
    if err == sql.ErrNoRows {
        err = ErrNoSuchUser
        return data, err
    }

    if len(since) < 1 {
        rows, err = serv.db.Query(
            `SELECT users.username, messages.timesent, messages.message
            FROM messages
            INNER JOIN users
            ON messages.sender = users.uid
            WHERE
            (sender = ? AND recipient = ?) OR (sender = ? AND recipient = ?)`,
            myUid, yourUid, yourUid, myUid)
    } else {
        rows, err = serv.db.Query(
            `SELECT users.username, messages.timesent, messages.message
            FROM messages
            INNER JOIN users
            ON messages.sender = users.uid
            WHERE
            ((sender = ? AND recipient = ?) OR (sender = ? AND recipient = ?)) AND
            (timesent > ?)`,
            myUid, yourUid, yourUid, myUid, []byte(since))
    }
    if err != nil {
        return data, err
    }

    buffer := new(bytes.Buffer)

    fmt.Fprint(buffer, "[")
    firstMsg := true
    for rows.Next() {
        if firstMsg {
            firstMsg = false
        } else {
            fmt.Fprint(buffer, ",")
        }

        var(
            username string
            timestamp string
            message []byte
        )

        if err = rows.Scan(&username, &timestamp, &message); err != nil {
            return data, err
        }

        fmt.Fprint(buffer, "{")
        fmt.Fprintf(buffer, `"Username": "%s",`, username)
        fmt.Fprintf(buffer, `"Timestamp": "%s",`, timestamp)
        fmt.Fprintf(buffer, `"Message": %s`, string(message))
        fmt.Fprint(buffer, "}")
    }
    fmt.Fprint(buffer, "]")
    data = buffer.Bytes()

    return data, err
}

func (serv *Server) LookupUser(user string) (uid int, err error) {
    row := serv.db.QueryRow(`select uid from users where username = ?`, user)
    err = row.Scan(&uid)
    if err == sql.ErrNoRows {
        err = ErrNoSuchUser
    }
    return
}

func (serv *Server) SendMsg(message []byte, receiver string, sender int) (err error) {
    row := serv.db.QueryRow(`select uid from users where username = ?`, receiver)

    var recipient int
    err = row.Scan(&recipient)
    if err == sql.ErrNoRows {
        err = ErrNoSuchUser
        return
    }

    _, err = serv.db.Exec(`insert into messages
            (sender, recipient, message, timesent)
            values (?, ?, ?, NOW())`,
            sender, recipient, message)
    if err != nil {
        err = ErrMsgUnsent
    }

    return
}

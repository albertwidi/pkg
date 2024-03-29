# Instrumentation

Instrumentation adds instrumentation context to the program with `Baggage` as the data.

The `Baggage` data is constructured based on several needs:

1. RequestID

    Is the unique request id generated by the server to identify a unique request. When this ID is propagated to other side of the program, we can add more context to our span/log/error/etc so we can correlate our events for ease of tracing and debugging.

1. APIName

    Is the name of api or request path. With this information we know what endpoints are being hit to add more context to our span/log/error/etc.

1. APIOwner

    Is the name of the owner for the API. We usually work in a team setup and when the team grows bigger, a certain team owns a set of APIs. When something goes wrong, this information can be used to escalte the problem into the correct team.

1. Forwarded-For

    Is the IP of the client(depends on how it passed). Usually forwarded-for contains client-ip, which also means it can be the real-ip of our user. This information, sometimes is important for debugging purpose and in-case of attack, we understand where the attack is coming from.

1. DebugID

    Is the special debug id generated internally to flag this is a debugging request to our system.

1. Preferred Language

    Is the preffered language of the user, so we can use the right language for the user to improve the user experience and ensure user understand our message.

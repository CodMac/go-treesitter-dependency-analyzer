package com.example.model;

import static java.util.concurrent.TimeUnit.DAYS;
import static java.util.concurrent.TimeUnit.HOURS;
import static java.util.concurrent.TimeUnit.MILLISECONDS;
import static java.util.concurrent.TimeUnit.MINUTES;
import static java.util.concurrent.TimeUnit.SECONDS;

import java.util.UUID;
import java.util.concurrent.TimeUnit;

public class User {
    // Contain: User CONTAIN Field (id, username)
    private String id;
    private String username;
    AddonInfo aInfo;

    // Contain: User CONTAIN Field (DEFAULT_ID)
    private static final String DEFAULT_ID = UUID.randomUUID().toString(); // Use: UUID.randomUUID()

    // Contain: User CONTAIN Constructor (User)
    public User(String username) {
        // Use: Field Access (this.id, this.username)
        this.id = DEFAULT_ID;
        this.username = username;
    }

    // Contain: User CONTAIN Method (getId)
    // Return: Method RETURN String Type
    public String getId() {
        return id;
    }

    // Contain: User CONTAIN Method (setUsername)
    // Parameter: Method PARAMETER String Type
    public void setUsername(String username) {
        this.username = username;
    }

    // 内部类
    public static class AddonInfo {
        public String otherName;
        private int age;
        protected java.util.Date birthday;
        String workTimeUnit;

        private static TimeUnit chooseUnit(long nanos) {
            if (DAYS.convert(nanos, MILLISECONDS) > 0) {
                return DAYS;
            }
            if (HOURS.convert(nanos, MILLISECONDS) > 0) {
                return HOURS;
            }
            if (MINUTES.convert(nanos, MILLISECONDS) > 0) {
                return MINUTES;
            }
            if (SECONDS.convert(nanos, MILLISECONDS) > 0) {
                return SECONDS;
            }
            return MILLISECONDS;
        }
    }
}
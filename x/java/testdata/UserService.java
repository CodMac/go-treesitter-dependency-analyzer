package com.example.service;

import com.example.model.User;
import java.util.List;
import java.io.IOException;

public class UserService {
    private final String dbUrl;

    public UserService(String url) {
        this.dbUrl = url;
    }

    public User createNewUser(String name) throws IOException {
        User u = new User(name); // CREATE and CALL (constructor)
        return u; // RETURN
    }

    public void processUsers(List<User> users) { // PARAMETER
        if (users != null) {
            System.out.println("Processing " + users.size() + " users."); // CALL
        }
    }
}
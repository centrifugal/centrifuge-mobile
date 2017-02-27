package com.example.fz.centrifugoandroid;

import android.support.v7.app.AppCompatActivity;
import android.os.Bundle;
import android.widget.TextView;

import centrifuge.Centrifuge;
import centrifuge.Client;
import centrifuge.Credentials;
import centrifuge.EventHandler;

public class MainActivity extends AppCompatActivity {

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);
        TextView tvId = (TextView) findViewById(R.id.text);

        Credentials creds = Centrifuge.newCredentials(
                "42", "1488055494", "",
                "24d0aa4d7c679e45e151d268044723d07211c6a9465d0e35ee35303d13c5eeff"
        );
        EventHandler events = Centrifuge.newEventHandler();

        Client client = Centrifuge.new_(
                "ws://192.168.1.37:8000/connection/websocket",
                creds,
                events,
                Centrifuge.defaultConfig()
        );

        try {
            client.connect();
        } catch (Exception e) {
            e.printStackTrace();
            tvId.setText(e.getMessage());
            return;
        }
        tvId.setText("Connected");
    }
}

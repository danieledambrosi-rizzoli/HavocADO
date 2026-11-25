#include <global.hpp>

#include <UserInterface/Dialogs/Listener.hpp>

#include <QFile>
#include <QApplication>
#include <QDialog>
#include <QGridLayout>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QSpacerItem>

using namespace HavocNamespace::HavocSpace;
using namespace HavocNamespace::UserInterface::Dialogs;

bool is_number( const std::string& s )
{
    std::string::const_iterator it = s.begin();
    while (it != s.end() && std::isdigit(*it)) ++it;
    return !s.empty() && it == s.end();
}

bool is_hex( const std::string& s )
{
    std::string::const_iterator it = s.begin();
    while ( it != s.end() && std::isxdigit( static_cast<unsigned char>( *it ) ) ) ++it;
    return !s.empty() && it == s.end();
}

NewListener::NewListener( QDialog* Dialog )
{
    ListenerDialog = Dialog;

    if ( ListenerDialog->objectName().isEmpty() )
        ListenerDialog->setObjectName( QString::fromUtf8( "ListenerWidget" ) );

    Dialog->setStyleSheet( FileRead( ":/stylesheets/Dialogs/Listener" ) );

    ListenerDialog->resize( 550, 600 );

    gridLayout = new QGridLayout( ListenerDialog );
    gridLayout->setObjectName(QString::fromUtf8("gridLayout"));

    ConfigBox = new QGroupBox( ListenerDialog );
    ConfigBox->setObjectName(QString::fromUtf8("ConfigBox"));

    gridLayout_2 = new QGridLayout( ConfigBox );
    gridLayout_2->setObjectName( QString::fromUtf8( "gridLayout_2" ) );
    gridLayout_2->setHorizontalSpacing( 0 );
    gridLayout_2->setContentsMargins( 0, 0, 0, 0 );

    StackWidgetConfigPages = new QStackedWidget( ConfigBox );
    StackWidgetConfigPages->setObjectName( QString::fromUtf8( "StackWidgetConfigPages" ) );

    // ============
    // === HTTP ===
    // ============
    PageHTTP = new QWidget();
    PageHTTP->setObjectName( QString::fromUtf8( "PageHTTP" ) );

    LabelHosts              = new QLabel( PageHTTP );
    HostsGroup              = new QGroupBox( PageHTTP );
    ButtonHostsGroupAdd     = new QPushButton( PageHTTP );
    ButtonHostsGroupClear   = new QPushButton( PageHTTP );

    LabelHostRotation       = new QLabel( PageHTTP );
    ComboHostRotation       = new QComboBox( PageHTTP );

    LabelHostBind           = new QLabel( PageHTTP );
    ComboHostBind           = new QComboBox( PageHTTP );

    LabelPortBind           = new QLabel( PageHTTP );
    InputPortBind           = new QLineEdit( PageHTTP );

    LabelPortConn           = new QLabel( PageHTTP );
    InputPortConn           = new QLineEdit( PageHTTP );

    LabelUserAgent          = new QLabel( PageHTTP );
    InputUserAgent          = new QLineEdit( PageHTTP );

    LabelHeaders            = new QLabel( PageHTTP );
    HeadersGroup            = new QGroupBox( PageHTTP );
    ButtonHeaderGroupAdd    = new QPushButton( PageHTTP );
    ButtonHeaderGroupClear  = new QPushButton( PageHTTP );

    LabelUris               = new QLabel( PageHTTP );
    UrisGroup               = new QGroupBox( PageHTTP );
    ButtonUriGroupClear     = new QPushButton( PageHTTP );
    ButtonUriGroupAdd       = new QPushButton( PageHTTP );

    LabelHostHeader         = new QLabel( PageHTTP );
    InputHostHeader         = new QLineEdit( PageHTTP );

    CheckEnableProxy        = new QCheckBox( PageHTTP );

    horizontalSpacer_6      = new QSpacerItem( 0, 0, QSizePolicy::Expanding, QSizePolicy::Minimum );
    verticalSpacer          = new QSpacerItem( 20, 40, QSizePolicy::Minimum, QSizePolicy::Expanding );
    verticalSpacerHeader    = new QSpacerItem( 20, 40, QSizePolicy::Minimum, QSizePolicy::Expanding );
    ProxyConfigBox          = new QGroupBox( PageHTTP );
    verticalSpacerUris      = new QSpacerItem( 20, 40, QSizePolicy::Minimum, QSizePolicy::Expanding );

    formLayout_Hosts        = new QFormLayout( HostsGroup );
    formLayout_Header       = new QFormLayout( HeadersGroup );
    formLayout_Uri          = new QFormLayout( UrisGroup );
    formLayout_3            = new QFormLayout( ProxyConfigBox );

    LabelProxyType = new QLabel( ProxyConfigBox );
    ComboProxyType = new QComboBox( ProxyConfigBox );
    LabelProxyHost = new QLabel( ProxyConfigBox );
    InputProxyHost = new QLineEdit( ProxyConfigBox );
    LabelProxyPort = new QLabel( ProxyConfigBox );
    InputProxyPort = new QLineEdit( ProxyConfigBox );
    LabelUserName  = new QLabel( ProxyConfigBox );
    InputUserName  = new QLineEdit( ProxyConfigBox );
    LabelPassword  = new QLabel( ProxyConfigBox );
    InputPassword  = new QLineEdit( ProxyConfigBox );

    formLayout_3->setWidget( 0, QFormLayout::LabelRole, LabelProxyType );
    formLayout_3->setWidget( 0, QFormLayout::FieldRole, ComboProxyType );
    formLayout_3->setWidget( 1, QFormLayout::LabelRole, LabelProxyHost );
    formLayout_3->setWidget( 1, QFormLayout::FieldRole, InputProxyHost );
    formLayout_3->setWidget( 2, QFormLayout::LabelRole, LabelProxyPort );
    formLayout_3->setWidget( 2, QFormLayout::FieldRole, InputProxyPort );
    formLayout_3->setWidget( 3, QFormLayout::LabelRole, LabelUserName );
    formLayout_3->setWidget( 3, QFormLayout::FieldRole, InputUserName );
    formLayout_3->setWidget( 4, QFormLayout::LabelRole, LabelPassword );
    formLayout_3->setWidget( 4, QFormLayout::FieldRole, InputPassword );

    ComboHostBind->addItems( QStringList() << HavocX::Teamserver.IpAddresses << "127.0.0.1" << "0.0.0.0" );

    CheckEnableProxy->setObjectName( "bool" );
    ProxyConfigBox->setEnabled( true );
    InputUserAgent->setText( "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36" ); // default. maybe make it dynamic/random ?
    InputUserAgent->setCursorPosition( 0 );
    InputPortBind->setText( "443" );
    InputPortConn->setText( "443" );

    // =============
    // ==== SMB ====
    // =============
    PageSMB = new QWidget();
    PageSMB->setObjectName(QString::fromUtf8("PageSMB"));
    formLayout = new QFormLayout( PageSMB );
    formLayout->setObjectName(QString::fromUtf8("formLayout"));
    LabelPipeName = new QLabel( PageSMB );
    LabelPipeName->setObjectName(QString::fromUtf8("LabelPipeName"));

    formLayout->setWidget(0, QFormLayout::LabelRole, LabelPipeName);

    InputPipeName = new QLineEdit( PageSMB );
    InputPipeName->setObjectName( QString::fromUtf8( "InputPipeName" ) );

    formLayout->setWidget(0, QFormLayout::FieldRole, InputPipeName);

    // ==============
    // == External ==
    // ==============
    PageExternal = new QWidget();
    PageExternal->setObjectName(QString::fromUtf8("PageExternal"));
    formLayout_2 = new QFormLayout(PageExternal);
    formLayout_2->setObjectName(QString::fromUtf8("formLayout_2"));
    LabelEndpoint = new QLabel(PageExternal);
    LabelEndpoint->setObjectName(QString::fromUtf8("LabelEndpoint"));

    formLayout_2->setWidget(0, QFormLayout::LabelRole, LabelEndpoint);

    InputEndpoint = new QLineEdit(PageExternal);
    InputEndpoint->setObjectName(QString::fromUtf8("InputEndpoint"));

    formLayout_2->setWidget(0, QFormLayout::FieldRole, InputEndpoint);

    // =============
    // ==== DNS ====
    // =============
    PageDNS = new QWidget();
    PageDNS->setObjectName(QString::fromUtf8("PageDNS"));
    formLayout_DNS = new QFormLayout(PageDNS);
    formLayout_DNS->setObjectName(QString::fromUtf8("formLayout_DNS"));

    LabelDomain = new QLabel(PageDNS);
    LabelDomain->setObjectName(QString::fromUtf8("LabelDomain"));
    formLayout_DNS->setWidget(0, QFormLayout::LabelRole, LabelDomain);

    InputDomain = new QLineEdit(PageDNS);
    InputDomain->setObjectName(QString::fromUtf8("InputDomain"));
    formLayout_DNS->setWidget(0, QFormLayout::FieldRole, InputDomain);

    LabelDNSBindHost = new QLabel(PageDNS);
    LabelDNSBindHost->setObjectName(QString::fromUtf8("LabelDNSBindHost"));
    formLayout_DNS->setWidget(1, QFormLayout::LabelRole, LabelDNSBindHost);

    InputDNSBindHost = new QLineEdit(PageDNS);
    InputDNSBindHost->setObjectName(QString::fromUtf8("InputDNSBindHost"));
    InputDNSBindHost->setText("0.0.0.0");
    formLayout_DNS->setWidget(1, QFormLayout::FieldRole, InputDNSBindHost);

    LabelDNSBindPort = new QLabel(PageDNS);
    LabelDNSBindPort->setObjectName(QString::fromUtf8("LabelDNSBindPort"));
    formLayout_DNS->setWidget(2, QFormLayout::LabelRole, LabelDNSBindPort);

    InputDNSBindPort = new QLineEdit(PageDNS);
    InputDNSBindPort->setObjectName(QString::fromUtf8("InputDNSBindPort"));
    InputDNSBindPort->setText("53");
    formLayout_DNS->setWidget(2, QFormLayout::FieldRole, InputDNSBindPort);

    LabelRecordType = new QLabel(PageDNS);
    LabelRecordType->setObjectName(QString::fromUtf8("LabelRecordType"));
    formLayout_DNS->setWidget(3, QFormLayout::LabelRole, LabelRecordType);

    ComboRecordType = new QComboBox(PageDNS);
    ComboRecordType->setObjectName(QString::fromUtf8("ComboRecordType"));
    ComboRecordType->addItem("TXT");
    formLayout_DNS->setWidget(3, QFormLayout::FieldRole, ComboRecordType);

    LabelEncoding = new QLabel(PageDNS);
    LabelEncoding->setObjectName(QString::fromUtf8("LabelEncoding"));
    formLayout_DNS->setWidget(4, QFormLayout::LabelRole, LabelEncoding);

    ComboEncoding = new QComboBox(PageDNS);
    ComboEncoding->setObjectName(QString::fromUtf8("ComboEncoding"));
    ComboEncoding->addItem("base64url");
    formLayout_DNS->setWidget(4, QFormLayout::FieldRole, ComboEncoding);

    LabelFrequency = new QLabel(PageDNS);
    LabelFrequency->setObjectName(QString::fromUtf8("LabelFrequency"));
    formLayout_DNS->setWidget(5, QFormLayout::LabelRole, LabelFrequency);

    InputFrequency = new QLineEdit(PageDNS);
    InputFrequency->setObjectName(QString::fromUtf8("InputFrequency"));
    InputFrequency->setText("30");
    formLayout_DNS->setWidget(5, QFormLayout::FieldRole, InputFrequency);

    LabelEncryptKey = new QLabel(PageDNS);
    LabelEncryptKey->setObjectName(QString::fromUtf8("LabelEncryptKey"));
    formLayout_DNS->setWidget(6, QFormLayout::LabelRole, LabelEncryptKey);

    InputEncryptKey = new QLineEdit(PageDNS);
    InputEncryptKey->setObjectName(QString::fromUtf8("InputEncryptKey"));
    InputEncryptKey->setPlaceholderText(QString::fromUtf8("empty = random key"));
    formLayout_DNS->setWidget(6, QFormLayout::FieldRole, InputEncryptKey);

    LabelPktSize = new QLabel(PageDNS);
    LabelPktSize->setObjectName(QString::fromUtf8("LabelPktSize"));
    formLayout_DNS->setWidget(7, QFormLayout::LabelRole, LabelPktSize);

    InputPktSize = new QLineEdit(PageDNS);
    InputPktSize->setObjectName(QString::fromUtf8("InputPktSize"));
    InputPktSize->setText("512");
    formLayout_DNS->setWidget(7, QFormLayout::FieldRole, InputPktSize);

    LabelTTL = new QLabel(PageDNS);
    LabelTTL->setObjectName(QString::fromUtf8("LabelTTL"));
    formLayout_DNS->setWidget(8, QFormLayout::LabelRole, LabelTTL);

    InputTTL = new QLineEdit(PageDNS);
    InputTTL->setObjectName(QString::fromUtf8("InputTTL"));
    InputTTL->setText("10");
    formLayout_DNS->setWidget(8, QFormLayout::FieldRole, InputTTL);

    LabelBurstEnabled = new QLabel(PageDNS);
    LabelBurstEnabled->setObjectName(QString::fromUtf8("LabelBurstEnabled"));
    formLayout_DNS->setWidget(9, QFormLayout::LabelRole, LabelBurstEnabled);

    CheckBurstEnabled = new QCheckBox(PageDNS);
    CheckBurstEnabled->setObjectName(QString::fromUtf8("CheckBurstEnabled"));
    formLayout_DNS->setWidget(9, QFormLayout::FieldRole, CheckBurstEnabled);

    LabelBurstSleep = new QLabel(PageDNS);
    LabelBurstSleep->setObjectName(QString::fromUtf8("LabelBurstSleep"));
    formLayout_DNS->setWidget(10, QFormLayout::LabelRole, LabelBurstSleep);

    InputBurstSleep = new QLineEdit(PageDNS);
    InputBurstSleep->setObjectName(QString::fromUtf8("InputBurstSleep"));
    InputBurstSleep->setText("50");
    formLayout_DNS->setWidget(10, QFormLayout::FieldRole, InputBurstSleep);

    LabelBurstJitter = new QLabel(PageDNS);
    LabelBurstJitter->setObjectName(QString::fromUtf8("LabelBurstJitter"));
    formLayout_DNS->setWidget(11, QFormLayout::LabelRole, LabelBurstJitter);

    InputBurstJitter = new QLineEdit(PageDNS);
    InputBurstJitter->setObjectName(QString::fromUtf8("InputBurstJitter"));
    InputBurstJitter->setText("0");
    formLayout_DNS->setWidget(11, QFormLayout::FieldRole, InputBurstJitter);

    LabelResolvers = new QLabel(PageDNS);
    LabelResolvers->setObjectName(QString::fromUtf8("LabelResolvers"));
    formLayout_DNS->setWidget(12, QFormLayout::LabelRole, LabelResolvers);

    InputResolvers = new QLineEdit(PageDNS);
    InputResolvers->setObjectName(QString::fromUtf8("InputResolvers"));
    InputResolvers->setText("8.8.8.8");
    formLayout_DNS->setWidget(12, QFormLayout::FieldRole, InputResolvers);

    LabelDohResolvers = new QLabel(PageDNS);
    LabelDohResolvers->setObjectName(QString::fromUtf8("LabelDohResolvers"));
    formLayout_DNS->setWidget(13, QFormLayout::LabelRole, LabelDohResolvers);

    InputDohResolvers = new QLineEdit(PageDNS);
    InputDohResolvers->setObjectName(QString::fromUtf8("InputDohResolvers"));
    formLayout_DNS->setWidget(13, QFormLayout::FieldRole, InputDohResolvers);

    gridLayout_2->addWidget( StackWidgetConfigPages, 0, 0, 1, 1 );


    gridLayout->addWidget(ConfigBox, 3, 0, 1, 6);

    ComboPayload = new QComboBox( ListenerDialog );
    ComboPayload->setObjectName( QString::fromUtf8( "ComboPayload" ) );

    gridLayout->addWidget(ComboPayload, 1, 1, 1, 5);

    LabelListenerName = new QLabel(ListenerDialog);
    LabelListenerName->setObjectName(QString::fromUtf8("LabelListenerName"));

    gridLayout->addWidget(LabelListenerName, 0, 0, 1, 1);

    LabelPayload = new QLabel(ListenerDialog);
    LabelPayload->setObjectName(QString::fromUtf8("LabelPayload"));

    gridLayout->addWidget(LabelPayload, 1, 0, 1, 1);

    horizontalSpacer_5 = new QSpacerItem(40, 20, QSizePolicy::Expanding, QSizePolicy::Minimum);

    gridLayout->addItem(horizontalSpacer_5, 4, 4, 1, 1);

    ButtonSave = new QPushButton(ListenerDialog);
    ButtonSave->setObjectName(QString::fromUtf8("ButtonSave"));

    gridLayout->addWidget(ButtonSave, 4, 2, 1, 1);

    horizontalSpacer_4 = new QSpacerItem(40, 20, QSizePolicy::Expanding, QSizePolicy::Minimum);

    gridLayout->addItem(horizontalSpacer_4, 4, 1, 1, 1);

    horizontalSpacer_3 = new QSpacerItem(40, 20, QSizePolicy::Expanding, QSizePolicy::Minimum);

    gridLayout->addItem(horizontalSpacer_3, 4, 5, 1, 1);

    InputListenerName = new QLineEdit(ListenerDialog);
    InputListenerName->setObjectName(QString::fromUtf8("InputListenerName"));

    gridLayout->addWidget(InputListenerName, 0, 1, 1, 5);

    horizontalSpacer_2 = new QSpacerItem(40, 20, QSizePolicy::Expanding, QSizePolicy::Minimum);

    gridLayout->addItem(horizontalSpacer_2, 4, 0, 1, 1);

    ButtonClose = new QPushButton(ListenerDialog);
    ButtonClose->setObjectName(QString::fromUtf8("ButtonClose"));

    gridLayout->addWidget(ButtonClose, 4, 3, 1, 1);

    horizontalSpacer = new QSpacerItem(40, 20, QSizePolicy::Expanding, QSizePolicy::Minimum);

    gridLayout->addItem(horizontalSpacer, 2, 0, 1, 6);

    StackWidgetConfigPages->setCurrentIndex( 0 );

    // Page HTTP/HTTPs
    gridLayout_3 = new QGridLayout( PageHTTP );
    gridLayout_3->setObjectName( QString::fromUtf8( "gridLayout_3" ) );

    gridLayout_3->addWidget( LabelUserAgent, 8, 0, 1, 1 );
    gridLayout_3->addWidget( ButtonUriGroupClear, 16, 2, 1, 1 );
    gridLayout_3->addWidget( ComboHostBind, 5, 1, 1, 2 );
    gridLayout_3->addWidget( LabelUris, 15, 0, 1, 1 );
    gridLayout_3->addWidget( LabelHostHeader, 19, 0, 1, 1 );
    gridLayout_3->addWidget( InputPortBind, 6, 1, 1, 2 );
    gridLayout_3->addWidget( InputPortConn, 7, 1, 1, 2 );
    gridLayout_3->addWidget( CheckEnableProxy, 20, 0, 1, 3 );
    gridLayout_3->addWidget( ButtonHeaderGroupClear, 11, 2, 1, 1 );
    gridLayout_3->addWidget( InputHostHeader, 19, 1, 1, 2 );
    gridLayout_3->addWidget( LabelHeaders, 10, 0, 1, 1 );
    gridLayout_3->addWidget( ButtonHostsGroupAdd, 0, 2, 1, 1 );
    gridLayout_3->addWidget( LabelHostBind, 5, 0, 1, 1 );
    gridLayout_3->addWidget( InputUserAgent, 8, 1, 1, 2 );
    gridLayout_3->addWidget( ButtonUriGroupAdd, 15, 2, 1, 1 );
    gridLayout_3->addWidget( HeadersGroup, 10, 1, 3, 1 );
    gridLayout_3->addWidget( LabelHosts, 0, 0, 1, 1 );
    gridLayout_3->addWidget( ButtonHostsGroupClear, 1, 2, 1, 1 );
    gridLayout_3->addWidget( ButtonHeaderGroupAdd, 10, 2, 1, 1 );
    gridLayout_3->addWidget( HostsGroup, 0, 1, 4, 1 );
    gridLayout_3->addWidget( LabelPortBind, 6, 0, 1, 1 );
    gridLayout_3->addWidget( LabelPortConn, 7, 0, 1, 1 );
    gridLayout_3->addWidget( ProxyConfigBox, 21, 0, 1, 3 );
    gridLayout_3->addWidget( UrisGroup, 15, 1, 3, 1 );
    gridLayout_3->addWidget( LabelHostRotation, 4, 0, 1, 1 );
    gridLayout_3->addWidget( ComboHostRotation, 4, 1, 1, 2 );
    gridLayout_3->addItem( horizontalSpacer_6, 18, 1, 1, 1 );
    gridLayout_3->addItem( verticalSpacer, 2, 0, 1, 1 );
    gridLayout_3->addItem( verticalSpacerHeader, 12, 0, 1, 1 );
    gridLayout_3->addItem( verticalSpacerUris, 17, 0, 1, 1 );

    ProxyConfigBox->setEnabled( false );

    InputProxyHost->setReadOnly( true );
    InputProxyPort->setReadOnly( true );
    InputUserName->setReadOnly( true );
    InputPassword->setReadOnly( true );

    InputProxyHost->setPlaceholderText( "" );

    LabelProxyHost->setEnabled( false );
    LabelProxyPort->setEnabled( false );
    LabelUserName->setEnabled( false );
    LabelPassword->setEnabled( false );

    auto style = QString( "color: #44475a;" );
    LabelProxyType->setStyleSheet( style );
    LabelProxyHost->setStyleSheet( style );
    LabelProxyPort->setStyleSheet( style );
    LabelUserName->setStyleSheet( style );
    LabelPassword->setStyleSheet( style );

    // Add Pages
    StackWidgetConfigPages->addWidget( PageHTTP );
    StackWidgetConfigPages->addWidget( PageSMB );
    StackWidgetConfigPages->addWidget( PageExternal );
    StackWidgetConfigPages->addWidget( PageDNS );

    ListenerDialog->setWindowTitle( "Create Listener" );
    LabelPayload->setText(QCoreApplication::translate("ListenerWidget", "Payload: ", nullptr));
    ComboPayload->setItemText(0, QCoreApplication::translate("ListenerWidget", "Https", nullptr));
    ComboPayload->setItemText(1, QCoreApplication::translate("ListenerWidget", "Http", nullptr));
    ComboPayload->setItemText(2, QCoreApplication::translate("ListenerWidget", "Smb", nullptr));
    ComboPayload->setItemText(3, QCoreApplication::translate("ListenerWidget", "External", nullptr));

    LabelListenerName->setText(QCoreApplication::translate("ListenerWidget", "Name:", nullptr));
    ButtonSave->setText(QCoreApplication::translate("ListenerWidget", "Save", nullptr));
    ButtonClose->setText(QCoreApplication::translate("ListenerWidget", "Close", nullptr));
    ConfigBox->setTitle(QCoreApplication::translate("ListenerWidget", "Config Options", nullptr));
    LabelUserAgent->setText(QCoreApplication::translate("ListenerWidget", "User Agent:  ", nullptr));
    ButtonUriGroupClear->setText(QCoreApplication::translate("ListenerWidget", "Clear", nullptr));
    LabelUris->setText(QCoreApplication::translate("ListenerWidget", "Uris:", nullptr));
    LabelHostHeader->setText(QCoreApplication::translate("ListenerWidget", "Host Header: ", nullptr));
    CheckEnableProxy->setText(QCoreApplication::translate("ListenerWidget", "Enable Proxy connection", nullptr));
    ButtonHeaderGroupClear->setText(QCoreApplication::translate("ListenerWidget", "Clear", nullptr));
    LabelHeaders->setText(QCoreApplication::translate("ListenerWidget", "Headers:", nullptr));
    ButtonHostsGroupAdd->setText(QCoreApplication::translate("ListenerWidget", "Add", nullptr));
    LabelHostBind->setText(QCoreApplication::translate("ListenerWidget", "Host (Bind):", nullptr));
    ButtonUriGroupAdd->setText(QCoreApplication::translate("ListenerWidget", "Add", nullptr));
    LabelHosts->setText(QCoreApplication::translate("ListenerWidget", "Hosts", nullptr));
    ButtonHostsGroupClear->setText(QCoreApplication::translate("ListenerWidget", "Clear", nullptr));
    ButtonHeaderGroupAdd->setText(QCoreApplication::translate("ListenerWidget", "Add", nullptr));
    LabelPortBind->setText(QCoreApplication::translate("ListenerWidget", "PortBind:", nullptr));
    LabelPortConn->setText(QCoreApplication::translate("ListenerWidget", "PortConn:", nullptr));
    LabelProxyType->setText(QCoreApplication::translate("ListenerWidget", "Proxy Type:", nullptr));
    LabelProxyHost->setText(QCoreApplication::translate("ListenerWidget", "Proxy Host:", nullptr));
    LabelProxyPort->setText(QCoreApplication::translate("ListenerWidget", "Proxy Port: ", nullptr));
    LabelUserName->setText(QCoreApplication::translate("ListenerWidget", "UserName: ", nullptr));
    LabelPassword->setText(QCoreApplication::translate("ListenerWidget", "Password: ", nullptr));
    LabelHostRotation->setText(QCoreApplication::translate("ListenerWidget", "Host Rotation: ", nullptr));
    LabelPipeName->setText(QCoreApplication::translate("ListenerWidget", "Pipe Name: ", nullptr));
    LabelEndpoint->setText(QCoreApplication::translate("ListenerWidget", "Endpoint: ", nullptr));
    LabelDomain->setText(QCoreApplication::translate("ListenerWidget", "Domain: ", nullptr));
    LabelDNSBindHost->setText(QCoreApplication::translate("ListenerWidget", "Bind Host: ", nullptr));
    LabelDNSBindPort->setText(QCoreApplication::translate("ListenerWidget", "Bind Port: ", nullptr));
    LabelRecordType->setText(QCoreApplication::translate("ListenerWidget", "Record Type: ", nullptr));
    LabelEncoding->setText(QCoreApplication::translate("ListenerWidget", "Encoding: ", nullptr));
    LabelFrequency->setText(QCoreApplication::translate("ListenerWidget", "Frequency (seconds): ", nullptr));
    LabelEncryptKey->setText(QCoreApplication::translate("ListenerWidget", "Encrypt Key (16 bytes): ", nullptr));
    LabelPktSize->setText(QCoreApplication::translate("ListenerWidget", "Packet Size: ", nullptr));
    LabelTTL->setText(QCoreApplication::translate("ListenerWidget", "TTL: ", nullptr));
    LabelBurstEnabled->setText(QCoreApplication::translate("ListenerWidget", "Burst Mode: ", nullptr));
    LabelBurstSleep->setText(QCoreApplication::translate("ListenerWidget", "Burst Sleep (ms): ", nullptr));
    LabelBurstJitter->setText(QCoreApplication::translate("ListenerWidget", "Burst Jitter (%): ", nullptr));
    LabelResolvers->setText(QCoreApplication::translate("ListenerWidget", "Resolvers: ", nullptr));
    LabelDohResolvers->setText(QCoreApplication::translate("ListenerWidget", "DoH Resolvers: ", nullptr));

    ComboPayload->addItem( "Https" );
    ComboPayload->addItem( "Http" );
    ComboPayload->addItem( "Smb" );
    ComboPayload->addItem( "External" );
    ComboPayload->addItem( "Dns" );

    ComboProxyType->addItem( "http" );
    ComboProxyType->addItem( "https" );

    ComboHostRotation->addItem( "round-robin" );
    ComboHostRotation->addItem( "random" );

    QObject::connect( ButtonSave, &QPushButton::clicked, this, &NewListener::onButton_Save );
    QObject::connect( ButtonClose, &QPushButton::clicked, this, [&]()
    {
        this->DialogClosed = true;
        this->ListenerDialog->close();

        // Free();
    } );

    QObject::connect( ButtonHostsGroupAdd, &QPushButton::clicked, this, [&]()
    {
        auto Item = new QLineEdit;
        Item->setFocus();

        if ( HostsData.size() == 0 ) {
            if ( ! HavocX::Teamserver.IpAddresses.isEmpty() ) {
                Item->setText( HavocX::Teamserver.IpAddresses[ 0 ] );
            }
        }

        formLayout_Hosts->setWidget( HostsData.size(), QFormLayout::FieldRole, Item );

        HostsData.push_back( Item );
        ListenerDialog->resize( 550, 500 );
    } );

    QObject::connect( ButtonHostsGroupClear, &QPushButton::clicked, this, [&]()
    {
        for ( auto& uri : HostsData )
            delete uri;

        HostsData.clear();

        ListenerDialog->resize( 550, 500 );
    } );

    QObject::connect( ButtonUriGroupAdd, &QPushButton::clicked, this, [&]()
    {
        auto Item = new QLineEdit;
        Item->setFocus();

        formLayout_Uri->setWidget( UrisData.size(), QFormLayout::FieldRole, Item );

        UrisData.push_back( Item );
        ListenerDialog->resize( 550, 500 );
    } );

    QObject::connect( ButtonUriGroupClear, &QPushButton::clicked, this, [&]()
    {
        for ( auto& uri : UrisData )
            delete uri;

        UrisData.clear();

        ListenerDialog->resize( 550, 500 );
    } );

    QObject::connect( ButtonHeaderGroupAdd, &QPushButton::clicked, this, [&]()
    {
        auto Item = new QLineEdit;
        Item->setFocus();

        formLayout_Header->setWidget( HeadersData.size(), QFormLayout::FieldRole, Item );

        HeadersData.push_back( Item );
        ListenerDialog->resize( 550, 500 );
    } );

    QObject::connect( ButtonHeaderGroupClear, &QPushButton::clicked, this, [&]()
    {
        for ( auto& header : HeadersData )
            delete header;

        HeadersData.clear();

        ListenerDialog->resize( 550, 500 );
    } );

    QObject::connect( ComboPayload, &QComboBox::currentTextChanged, this, [&]( const QString& text )
    {
        if ( text.compare( HavocSpace::Listener::PayloadHTTPS ) == 0 )
        {
            StackWidgetConfigPages->setCurrentIndex( 0 );
            InputPortBind->setText( "443" );
            InputPortConn->setText( "443" );
        }
        else if ( text.compare( HavocSpace::Listener::PayloadHTTP ) == 0 )
        {
            StackWidgetConfigPages->setCurrentIndex( 0 );
            InputPortBind->setText( "80" );
            InputPortConn->setText( "80" );
        }
        else if ( text.compare( HavocSpace::Listener::PayloadSMB ) == 0 )
        {
            StackWidgetConfigPages->setCurrentIndex( 1 );
        }
        else if ( text.compare( HavocSpace::Listener::PayloadExternal ) == 0 )
        {
            StackWidgetConfigPages->setCurrentIndex( 2 );
        }
        else if ( text.compare( HavocSpace::Listener::PayloadDNS ) == 0 )
        {
            StackWidgetConfigPages->setCurrentIndex( 3 );
        }
        else
        {
            for ( const auto& listener : ServiceListeners )
            {
                if ( listener.Name == text.toStdString() )
                {
                    StackWidgetConfigPages->setCurrentIndex( listener.Index );
                    return;
                }
            }

            spdlog::error( "Payload not found" );
        }
    } );

    QObject::connect( CheckEnableProxy, &QCheckBox::toggled, this, &NewListener::onProxyEnabled );

    QMetaObject::connectSlotsByName( Dialog );
}

MapStrStr NewListener::Start( Util::ListenerItem Item, bool Edit )
{
    auto ListenerInfo = MapStrStr();
    auto Payload      = QString();

    if ( Edit )
    {
        InputListenerName->setText( Item.Name.c_str() );
        InputListenerName->setReadOnly( true );

        if ( ( Item.Protocol == Listener::PayloadHTTP.toStdString() ) || ( Item.Protocol == Listener::PayloadHTTPS.toStdString() ) )
        {
            if ( Item.Protocol == Listener::PayloadHTTPS.toStdString() )
                ComboPayload->setCurrentIndex( 0 );
            else
                ComboPayload->setCurrentIndex( 1 );

            ComboPayload->setDisabled( true );

            auto Info = any_cast<Listener::HTTP>( Item.Info );

            ComboHostBind->addItem( Info.HostBind );
            ComboHostBind->setDisabled( true );

            if ( Info.HostRotation.compare( "round-robin" ) == 0 )
                ComboHostRotation->setCurrentIndex( 0 );
            else if ( Info.HostRotation.compare( "random" ) == 0 )
                ComboHostRotation->setCurrentIndex( 1 );
            else
                ComboHostRotation->setCurrentIndex( 0 );

            InputPortBind->setText( Info.PortBind );
            InputPortBind->setReadOnly( true );

            InputPortConn->setText( Info.PortConn );
            InputPortConn->setReadOnly( true );

            InputUserAgent->setText( Info.UserAgent );
            InputUserAgent->setCursorPosition( 0 );

            if ( ! Info.Hosts.empty() )
            {
                for ( const auto& host : Info.Hosts )
                {
                    if ( host.isEmpty() )
                        continue;

                    auto input = new QLineEdit;
                    input->setText( host );

                    formLayout_Hosts->setWidget( HostsData.size(), QFormLayout::FieldRole, input );

                    HostsData.push_back( input );
                }
            }

            if ( ! Info.Headers.empty() )
            {
                for ( const auto& header : Info.Headers )
                {
                    if ( header.isEmpty() )
                        continue;

                    auto input = new QLineEdit;
                    input->setText( header );

                    formLayout_Header->setWidget( HeadersData.size(), QFormLayout::FieldRole, input );

                    HeadersData.push_back( input );
                }
            }

            if ( ! Info.Uris.empty() )
            {
                for ( const auto& uri : Info.Uris )
                {
                    if ( uri.isEmpty() )
                        continue;

                    auto input = new QLineEdit;
                    input->setText(uri );

                    formLayout_Uri->setWidget(UrisData.size(), QFormLayout::FieldRole, input );

                    UrisData.push_back(input );
                }
            }

            InputHostHeader->setText( Info.HostHeader );

            if ( Info.ProxyEnabled.compare( "true" ) == 0 )
                CheckEnableProxy->setCheckState( Qt::CheckState::Checked );
            else
                CheckEnableProxy->setCheckState( Qt::CheckState::Unchecked );

            if ( Info.ProxyType.compare( "http" ) == 0 )
                ComboProxyType->setCurrentIndex( 0 );
            else
                ComboProxyType->setCurrentIndex( 1 );

            InputProxyHost->setText( Info.ProxyHost );
            InputProxyPort->setText( Info.ProxyPort );
            InputUserName->setText( Info.ProxyUsername );
            InputPassword->setText( Info.ProxyPassword );
        }
        else if ( Item.Protocol == Listener::PayloadSMB.toStdString() )
        {
            ComboPayload->setCurrentIndex( 2 );
            auto Info = any_cast<Listener::SMB>( Item.Info );

            InputPipeName->setText( Info.PipeName );
            InputPipeName->setReadOnly( true );
        }
        else if ( Item.Protocol == Listener::PayloadExternal.toStdString() )
        {
            ComboPayload->setCurrentIndex( 3 );

            auto Info = any_cast<Listener::External>( Item.Info );

            InputEndpoint->setText( Info.Endpoint );
            InputEndpoint->setReadOnly( true );
        }
        else if ( Item.Protocol == Listener::PayloadDNS.toStdString() )
        {
            ComboPayload->setCurrentIndex( 4 );

            auto Info = any_cast<Listener::DNS>( Item.Info );

            InputDomain->setText( Info.Domain );
            InputDNSBindHost->setText( Info.BindHost );
            InputDNSBindPort->setText( Info.BindPort );
            InputFrequency->setText( Info.Frequency );

            if ( ! Info.EncryptKey.isEmpty() )
                InputEncryptKey->setText( Info.EncryptKey );
            if ( ! Info.PktSize.isEmpty() )
                InputPktSize->setText( Info.PktSize );
            if ( ! Info.TTL.isEmpty() )
                InputTTL->setText( Info.TTL );
            if ( ! Info.BurstSleep.isEmpty() )
                InputBurstSleep->setText( Info.BurstSleep );
            if ( ! Info.BurstJitter.isEmpty() )
                InputBurstJitter->setText( Info.BurstJitter );
            if ( ! Info.Resolvers.isEmpty() )
                InputResolvers->setText( Info.Resolvers );
            if ( ! Info.DohResolvers.isEmpty() )
                InputDohResolvers->setText( Info.DohResolvers );

            CheckBurstEnabled->setChecked( Info.BurstEnabled == "true" || Info.BurstEnabled == "1" );

            if ( ! Info.RecordType.isEmpty() )
                ComboRecordType->setCurrentText( Info.RecordType );
            if ( ! Info.Encoding.isEmpty() )
                ComboEncoding->setCurrentText( Info.Encoding );

            // The bind address can only be changed by restarting the listener.
            InputDNSBindHost->setReadOnly( true );
            InputDNSBindPort->setReadOnly( true );
        }
        else
        {
            // we assume that it's a service listener

            for ( const auto& listener : ServiceListeners )
            {
                if ( listener.Name == Item.Protocol )
                {
                    auto ListenerConfiguration = json::parse( any_cast<Listener::Service>( Item.Info )[ "Info" ] );

                    ComboPayload->setCurrentIndex( listener.Index + 1 );

                    /* TODO: iterate over ServiceListeners and check what has been set
                     *       and blah blah blah just set everything based on the specified object
                     *       and check if its editable etc. */

                    for ( const auto& item : listener.Items )
                    {
                        auto object   = item[ "object" ].get<std::string>();
                        auto editable = item[ "editable" ].get<bool>();
                        auto value    = QString();

                        value = QString( ListenerConfiguration[ item[ "name" ] ].get<std::string>().c_str() );

                        /* if object type is "input" */
                        if ( object == "input" )
                        {
                            auto Line = ( ( QLineEdit* ) item[ "Line" ].get<::uint64_t>() );

                            Line->setText( value );

                            if ( ! editable )
                                Line->setReadOnly( true );
                        }
                    }

                }
            }
        }

        ListenerDialog->setWindowTitle( "Edit Listener" );
        ComboPayload->setDisabled( true );
    }

    ListenerDialog->exec();

    Payload = ComboPayload->currentText();

    ListenerInfo.insert( { "Name",     InputListenerName->text().toStdString() } );
    ListenerInfo.insert( { "Protocol", ComboPayload->currentText().toStdString() } );
    ListenerInfo.insert( { "Status",  "online" } );

    if ( ( Payload.compare( HavocSpace::Listener::PayloadHTTPS ) == 0 ) || ( Payload.compare( HavocSpace::Listener::PayloadHTTP ) == 0 ) )
    {
        auto Hosts   = std::string();
        auto Headers = std::string();
        auto Uris    = std::string();

        if ( Payload.compare( HavocSpace::Listener::PayloadHTTPS ) == 0 )
            ListenerInfo.insert( { "Secure", "true"  } );
        else
            ListenerInfo.insert( { "Secure", "false" } );

        if ( ! HostsData.empty() )
        {
            for ( u32 i = 0; i < HostsData.size(); ++i )
            {
                if ( i == ( HostsData.size() - 1 ) )
                    Hosts += HostsData.at( i )->text().toStdString();
                else
                    Hosts += HostsData.at( i )->text().toStdString() + ", ";

                delete HostsData.at( i );
            }
        }
        else
        {
            Hosts = ComboHostBind->currentText().toStdString();
        }

        if ( ! HeadersData.empty() )
        {
            for ( u32 i = 0; i < HeadersData.size(); ++i )
            {
                if ( i == ( HeadersData.size() - 1 ) )
                    Headers += HeadersData.at( i )->text().toStdString();
                else
                    Headers += HeadersData.at( i )->text().toStdString() + ", ";

                delete HeadersData.at( i );
            }
        }

        if ( ! UrisData.empty() )
        {
            for ( u32 i = 0; i < UrisData.size(); ++i )
            {
                if ( i == ( UrisData.size() - 1 ) )
                    Uris += UrisData.at( i )->text().toStdString();
                else
                    Uris += UrisData.at( i )->text().toStdString() + ", ";

                delete UrisData.at( i );
            }
        }

        ListenerInfo.insert( { "Hosts", Hosts } );
        ListenerInfo.insert( { "HostBind", ComboHostBind->currentText().toStdString() } );
        ListenerInfo.insert( { "HostRotation", ComboHostRotation->currentText().toStdString() } );
        ListenerInfo.insert( { "PortBind", InputPortBind->text().toStdString() } );
        ListenerInfo.insert( { "PortConn", InputPortConn->text().toStdString() } );
        ListenerInfo.insert( { "Headers", Headers } );
        ListenerInfo.insert( { "Uris", Uris } );
        ListenerInfo.insert( { "UserAgent", InputUserAgent->text().toStdString() } );
        ListenerInfo.insert( { "HostHeader", InputHostHeader->text().toStdString() } );

        ListenerInfo.insert( { "Proxy Enabled", CheckEnableProxy->isChecked() ? "true" : "false" } );

        if ( CheckEnableProxy->isChecked() )
        {
            ListenerInfo.insert( { "Proxy Type", ComboProxyType->currentText().toStdString() } );
            ListenerInfo.insert( { "Proxy Host", InputProxyHost->text().toStdString() } );
            ListenerInfo.insert( { "Proxy Port", InputProxyPort->text().toStdString() } );
            ListenerInfo.insert( { "Proxy Username", InputUserName->text().toStdString() } );
            ListenerInfo.insert( { "Proxy Password", InputPassword->text().toStdString() } );
        }
    }
    else if ( Payload.compare( HavocSpace::Listener::PayloadSMB ) == 0 )
    {
        ListenerInfo.insert( { "PipeName", InputPipeName->text().toStdString() } );
    }
    else if ( Payload.compare( HavocSpace::Listener::PayloadExternal ) == 0 )
    {
        for ( auto& Listener : HavocX::Teamserver.Listeners )
        {
            if ( Listener.Protocol == HavocSpace::Listener::PayloadExternal.toStdString() )
            {
                if ( any_cast<HavocSpace::Listener::External>( Listener.Info ).Endpoint.compare( InputEndpoint->text() ) == 0 )
                {
                    MessageBox( "Listener Error", "Listener External: Endpoint already registered.", QMessageBox::Icon::Critical );
                    return MapStrStr{};
                }
            }
        }

        ListenerInfo.insert( { "Endpoint", InputEndpoint->text().toStdString() } );
    }
    else if ( Payload.compare( HavocSpace::Listener::PayloadDNS ) == 0 )
    {
        ListenerInfo.insert( { "Domain",     InputDomain->text().toStdString() } );
        ListenerInfo.insert( { "BindHost",   InputDNSBindHost->text().toStdString() } );
        ListenerInfo.insert( { "BindPort",   InputDNSBindPort->text().toStdString() } );
        ListenerInfo.insert( { "RecordType", ComboRecordType->currentText().toStdString() } );
        ListenerInfo.insert( { "Encoding",   ComboEncoding->currentText().toStdString() } );
        ListenerInfo.insert( { "Frequency",  InputFrequency->text().toStdString() } );
        ListenerInfo.insert( { "EncryptKey", InputEncryptKey->text().toStdString() } );
        ListenerInfo.insert( { "PktSize",    InputPktSize->text().toStdString() } );
        ListenerInfo.insert( { "TTL",        InputTTL->text().toStdString() } );
        ListenerInfo.insert( { "BurstEnabled", CheckBurstEnabled->isChecked() ? "true" : "false" } );
        ListenerInfo.insert( { "BurstSleep", InputBurstSleep->text().toStdString() } );
        ListenerInfo.insert( { "BurstJitter", InputBurstJitter->text().toStdString() } );
        ListenerInfo.insert( { "Resolvers",  InputResolvers->text().toStdString() } );
        ListenerInfo.insert( { "DohResolvers", InputDohResolvers->text().toStdString() } );
    }
    else
    {
        for ( const auto& listener : ServiceListeners )
        {
            if ( listener.Name == Payload.toStdString() )
            {
                auto Listener = MapStrStr{
                    { "Name",       InputListenerName->text().toStdString() },
                    { "Protocol",   listener.Name },
                    { "ClientUser", HavocX::Teamserver.User.toStdString() },
                };

                for ( const auto& item : listener.Items )
                {
                    auto object = QString( item[ "object" ].get<std::string>().c_str() );

                    if ( object == "input" )
                    {
                        auto Name = item[ "name" ].get<std::string>();
                        auto Line = ( QLineEdit* ) item[ "Line" ].get<::uint64_t>();

                        Listener.insert( { Name, Line->text().toStdString() } );
                    }
                }

                return Listener;
            }
        }

        spdlog::error( "Payload not found" );

        return {};
    }

    return ListenerInfo;
}

auto NewListener::ListenerCustomAdd( QString Json ) -> bool
{
    if ( Json.isEmpty() )
        return false;

    auto Listener = json::parse( Json.toStdString() );
    auto Page     = ( QWidget* )     nullptr;
    auto Layout   = ( QFormLayout* ) nullptr;
    auto Service  = ServiceListener();

    Page    = new QWidget;
    Layout  = new QFormLayout( Page );
    Service = {
        .Name   = Listener[ "Name" ],
        .Page   = Page,
        .Layout = Layout,
        .Index  = StackWidgetConfigPages->count()
    };

    for ( auto Item : Listener[ "Items" ] )
    {
        if ( Item[ "object" ] == "input" )
        {
            auto Label = new QLabel( Page );
            auto Line  = new QLineEdit( Page );
            auto index = Service.Items.size();

            Label->setText( Item[ "text" ].get<std::string>().c_str() );
            Line->setPlaceholderText( Item[ "placeholder" ].get<std::string>().c_str() );

            Layout->setWidget( index, QFormLayout::LabelRole, Label );
            Layout->setWidget( index, QFormLayout::FieldRole, Line  );

            Service.Items.push_back( {
                { "name",     Item[ "name" ]     },
                { "object",   Item[ "object" ]   },
                { "required", Item[ "required" ] },
                { "editable", Item[ "editable" ] },
                { "Label",    ( uint64_t ) Label },
                { "Line",     ( uint64_t ) Line  },
            } );
        }
    }

    ServiceListeners.push_back( Service );
    ComboPayload->addItem( Service.Name.c_str() );
    StackWidgetConfigPages->addWidget( Page );

    /* check if we already registered this listener */
    for ( auto& x : HavocX::Teamserver.RegisteredListeners )
    {
        if ( x[ "Name" ] == Listener[ "Name" ] )
            return false;
    }

    /* if not then lets add it. */
    HavocX::Teamserver.RegisteredListeners.push_back( Listener );

    return true;
}

void HavocNamespace::UserInterface::Dialogs::NewListener::onButton_Save()
{
    auto Payload = ComboPayload->currentText();

    if ( ( Payload.compare( HavocSpace::Listener::PayloadHTTPS ) == 0 ) ||
         ( Payload.compare( HavocSpace::Listener::PayloadHTTP  ) == 0 ) )
    {
        if ( InputListenerName->text().isEmpty() )
        {
            MessageBox( "Listener Error", "No Listener Name specified", QMessageBox::Critical );

            return;
        }

        if ( InputPortBind->text().isEmpty() )
        {
            MessageBox( "Listener Error", "No PortBind specified", QMessageBox::Critical );

            return;
        }
        else
        {
            if ( ! is_number( InputPortBind->text().toStdString() ) )
            {
                MessageBox( "Listener Error", "PortBind is not a number", QMessageBox::Critical );

                return;
            }
        }

        if ( InputPortConn->text().isEmpty() )
        {
            MessageBox( "Listener Error", "No PortConn specified", QMessageBox::Critical );

            return;
        }
        else
        {
            if ( ! is_number( InputPortConn->text().toStdString() ) )
            {
                MessageBox( "Listener Error", "PortConn is not a number", QMessageBox::Critical );

                return;
            }
        }

        if ( InputUserAgent->text().isEmpty() )
        {
            MessageBox( "Listener Error", "No UserAgent specified", QMessageBox::Critical );

            return;
        }

        if ( CheckEnableProxy->isChecked() )
        {
            if ( InputProxyHost->text().isEmpty() )
            {
                MessageBox( "Listener Error", "No Proxy Host specified", QMessageBox::Critical );

                return;
            }

            if ( InputProxyPort->text().isEmpty() )
            {
                MessageBox( "Listener Error", "No Proxy Port specified", QMessageBox::Critical );

                return;
            }
            else
            {
                if ( ! is_number( InputProxyPort->text().toStdString() ) )
                {
                    MessageBox( "Listener Error", "Port is not a number", QMessageBox::Critical );
                    return;
                }
            }
        }

    }
    else if ( Payload.compare( HavocSpace::Listener::PayloadSMB )  == 0 )
    {
        if ( InputPipeName->text().isEmpty() )
        {
            MessageBox( "Listener Error", "No Pipe name specified", QMessageBox::Critical );

            return;
        }
    }
    else if ( Payload.compare( HavocSpace::Listener::PayloadExternal )  == 0 )
    {
        if ( InputEndpoint->text().isEmpty() )
        {
            MessageBox( "Listener Error", "No Endpoint specified", QMessageBox::Critical );

            return;
        }
    }
    else if ( Payload.compare( HavocSpace::Listener::PayloadDNS ) == 0 )
    {
        if ( InputListenerName->text().isEmpty() )
        {
            MessageBox( "Listener Error", "No Listener Name specified", QMessageBox::Critical );

            return;
        }

        if ( InputDomain->text().isEmpty() )
        {
            MessageBox( "Listener Error", "No Domain specified", QMessageBox::Critical );

            return;
        }

        if ( InputDomain->text().contains( " " ) || ! InputDomain->text().contains( '.' ) )
        {
            MessageBox( "Listener Error", "Domain is not a valid FQDN", QMessageBox::Critical );

            return;
        }

        if ( InputDNSBindHost->text().isEmpty() )
        {
            MessageBox( "Listener Error", "No Bind Host specified", QMessageBox::Critical );

            return;
        }

        if ( InputDNSBindPort->text().isEmpty() || ! is_number( InputDNSBindPort->text().toStdString() ) )
        {
            MessageBox( "Listener Error", "Bind Port is not a number", QMessageBox::Critical );

            return;
        }

        if ( InputDNSBindPort->text().toInt() < 1 || InputDNSBindPort->text().toInt() > 65535 )
        {
            MessageBox( "Listener Error", "Bind Port is out of range", QMessageBox::Critical );

            return;
        }

        if ( InputFrequency->text().isEmpty() || ! is_number( InputFrequency->text().toStdString() ) )
        {
            MessageBox( "Listener Error", "Frequency is not a number", QMessageBox::Critical );

            return;
        }

        if ( InputFrequency->text().toInt() < 5 || InputFrequency->text().toInt() > 300 )
        {
            MessageBox( "Listener Error", "Frequency must be between 5 and 300 seconds", QMessageBox::Critical );

            return;
        }

        /* The encryption key is either 32 hex characters or 16 raw characters.
         * An empty key lets the teamserver generate a random one. */
        if ( ! InputEncryptKey->text().isEmpty() )
        {
            auto Key = InputEncryptKey->text().toStdString();

            if ( Key.size() != 16 && Key.size() != 32 )
            {
                MessageBox( "Listener Error", "Encrypt Key must be 16 bytes (32 hex characters)", QMessageBox::Critical );

                return;
            }

            if ( Key.size() == 32 && ! is_hex( Key ) )
            {
                MessageBox( "Listener Error", "Encrypt Key is not a valid hex string", QMessageBox::Critical );

                return;
            }
        }

        if ( InputPktSize->text().isEmpty() || ! is_number( InputPktSize->text().toStdString() ) )
        {
            MessageBox( "Listener Error", "Packet Size is not a number", QMessageBox::Critical );

            return;
        }

        if ( InputPktSize->text().toInt() < 64 || InputPktSize->text().toInt() > 4096 )
        {
            MessageBox( "Listener Error", "Packet Size must be between 64 and 4096 bytes", QMessageBox::Critical );

            return;
        }

        if ( InputTTL->text().isEmpty() || ! is_number( InputTTL->text().toStdString() ) )
        {
            MessageBox( "Listener Error", "TTL is not a number", QMessageBox::Critical );

            return;
        }

        if ( InputTTL->text().toInt() < 1 || InputTTL->text().toInt() > 86400 )
        {
            MessageBox( "Listener Error", "TTL must be between 1 and 86400 seconds", QMessageBox::Critical );

            return;
        }

        if ( InputBurstSleep->text().isEmpty() || ! is_number( InputBurstSleep->text().toStdString() ) )
        {
            MessageBox( "Listener Error", "Burst Sleep is not a number", QMessageBox::Critical );

            return;
        }

        if ( InputBurstSleep->text().toInt() < 10 || InputBurstSleep->text().toInt() > 60000 )
        {
            MessageBox( "Listener Error", "Burst Sleep must be between 10 and 60000 milliseconds", QMessageBox::Critical );

            return;
        }

        if ( InputBurstJitter->text().isEmpty() || ! is_number( InputBurstJitter->text().toStdString() ) )
        {
            MessageBox( "Listener Error", "Burst Jitter is not a number", QMessageBox::Critical );

            return;
        }

        if ( InputBurstJitter->text().toInt() < 0 || InputBurstJitter->text().toInt() > 90 )
        {
            MessageBox( "Listener Error", "Burst Jitter must be between 0 and 90 percent", QMessageBox::Critical );

            return;
        }
    }
    else
    {
        for ( const auto& listener : ServiceListeners )
        {
            if ( Payload.compare( listener.Name.c_str() ) == 0 )
            {
                for ( auto item : listener.Items )
                {
                    auto object = item[ "object" ].get<std::string>();

                    /* if object type is "input" */
                    if ( object == "input" )
                    {
                        auto Line = ( ( QLineEdit* ) item[ "Line" ].get<::uint64_t>() );

                        /* if the operator didn't specify a value that is required then let that operator know. */
                        if ( item[ "required" ].get<bool>() && Line->text().isEmpty() )
                        {
                            auto itemName = QString( item[ "name" ].get<std::string>().c_str() );
                            MessageBox( "Listener Error", "No " + itemName + " specified", QMessageBox::Critical );
                            return;
                        }
                    }

                }
            }
        }
    }

    this->DialogSaved = true;
    this->ListenerDialog->close();
}

void HavocNamespace::UserInterface::Dialogs::NewListener::onProxyEnabled()
{
    if ( CheckEnableProxy->isChecked() )
    {
        ProxyConfigBox->setEnabled( true );

        auto style = QString( "color: #f8f8f2;" );
        LabelProxyType->setStyleSheet( style );
        LabelProxyHost->setStyleSheet( style );
        LabelProxyPort->setStyleSheet( style );
        LabelUserName->setStyleSheet( style );
        LabelPassword->setStyleSheet( style );

        InputProxyHost->setReadOnly( false );
        InputProxyPort->setReadOnly( false );
        InputUserName->setReadOnly( false );
        InputPassword->setReadOnly( false );

        LabelProxyHost->setEnabled( false );
        LabelProxyPort->setEnabled( false );
        LabelUserName->setEnabled( false );
        LabelPassword->setEnabled( false );
    }
    else
    {
        ProxyConfigBox->setEnabled( false );

        auto style = QString( "color: #44475a;" );
        LabelProxyType->setStyleSheet( style );
        LabelProxyHost->setStyleSheet( style );
        LabelProxyPort->setStyleSheet( style );
        LabelUserName->setStyleSheet( style );
        LabelPassword->setStyleSheet( style );

        InputProxyHost->setReadOnly( true );
        InputProxyPort->setReadOnly( true );
        InputUserName->setReadOnly( true );
        InputPassword->setReadOnly( true );

        LabelProxyHost->setEnabled( true );
        LabelProxyPort->setEnabled( true );
        LabelUserName->setEnabled( true );
        LabelPassword->setEnabled( true );
    }
}

auto NewListener::Free() -> void
{
    for ( auto listener : ServiceListeners )
    {
        for ( auto item : listener.Items )
        {
            // delete ( QLabel* )    listener.Items[ item ][ "Label" ].get<uint64_t>();
            // delete ( QLineEdit* ) listener.Items[ item ][ "Line"  ].get<uint64_t>();
        }

        delete listener.Layout;
        delete listener.Page;
    }
}
